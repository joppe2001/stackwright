package registry

import (
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/joppe2001/stackwright/internal/config"
	"gopkg.in/yaml.v3"
)

// RegistryURL is the canonical GitHub-hosted registry source. Matches the
// signature tree (owner: joppe2001, repo: stackwright-registry).
const RegistryURL = "https://raw.githubusercontent.com/joppe2001/stackwright-registry/main/registry.json"

// Bundled is the catalog compiled into the binary. When the network is
// unreachable and no cache exists, this is what the TUI shows. JSON is valid
// YAML, so yaml.v3 parses it just fine.
//
//go:embed bundled.json
var bundledJSON []byte

// cacheMaxAge is how long a successfully-downloaded registry stays trusted.
// Older caches trigger a refetch; network failure falls back to the cache
// regardless of age.
const cacheMaxAge = 24 * time.Hour

// fetchTimeout is the spec-mandated 3-second deadline for the HTTP GET.
// We also use it as the dial timeout so DNS/connect failures fail fast.
const fetchTimeout = 3 * time.Second

// Source records where the loaded catalog came from. Exposed to the TUI so it
// can show a status indicator ("● live" vs "● offline — cached 2h ago").
type Source int

const (
	SourceBundled Source = iota
	SourceCache
	SourceNetwork
)

func (s Source) String() string {
	switch s {
	case SourceBundled:
		return "bundled"
	case SourceCache:
		return "cache"
	case SourceNetwork:
		return "network"
	default:
		return "unknown"
	}
}

// LoadOptions controls where Load() pulls data from. Offline forces the
// bundled+cache path; ForceRefresh bypasses the cache-age check.
type LoadOptions struct {
	Offline      bool
	ForceRefresh bool
}

// LoadResult is the catalog plus metadata about how it was obtained.
type LoadResult struct {
	Bundle        Bundle
	Source        Source
	CacheAge      time.Duration
	NetworkError  error // non-nil when we tried the network and failed
	LocalEntries  int   // count of entries pulled from registry.local.yaml
}

// Load produces the full registry by:
//  1. Starting with the bundled catalog.
//  2. Overlaying the freshest remote snapshot available (network or cache).
//  3. Overlaying registry.local.yaml on top.
//
// Never returns an error: every phase has a graceful fallback. Callers that
// want to warn about a network failure can inspect result.NetworkError.
func Load(opts LoadOptions) LoadResult {
	result := LoadResult{}

	// Start with bundled — always present, always parseable.
	bundled, err := parseBundle(bundledJSON)
	if err != nil {
		// bundled.json is checked in; if it fails to parse that's a build bug,
		// not a runtime bug. Surface loudly rather than silently falling through.
		panic(fmt.Errorf("stackwright: bundled registry failed to parse: %w", err))
	}
	result.Bundle = bundled
	result.Source = SourceBundled

	// Try remote (network or cache) unless offline.
	if !opts.Offline {
		remote, src, age, netErr := loadRemote(opts.ForceRefresh)
		result.NetworkError = netErr
		if remote != nil {
			result.Bundle = mergeByslug(result.Bundle, *remote)
			result.Source = src
			result.CacheAge = age
		}
	}

	// Overlay local user additions.
	if local, err := ReadLocal(); err == nil && local != nil {
		result.Bundle = mergeByslug(result.Bundle, *local)
		result.LocalEntries = len(local.Entries)
	}

	return result
}

// loadRemote is the network-or-cache phase. Returns (bundle, source, cacheAge, netErr).
// netErr is only non-nil when a network attempt was made AND failed; it's nil
// when we deliberately skip the network because the cache is fresh.
func loadRemote(force bool) (*Bundle, Source, time.Duration, error) {
	cachePath := config.RegistryCachePath()

	// Respect a fresh cache before hitting the network.
	if !force {
		if bundle, age, ok := readFreshCache(cachePath); ok {
			return &bundle, SourceCache, age, nil
		}
	}

	// Attempt network.
	body, err := fetchOverNetwork(RegistryURL, fetchTimeout)
	if err != nil {
		// Network failed — fall back to any cache regardless of age.
		if bundle, age, ok := readAnyCache(cachePath); ok {
			return &bundle, SourceCache, age, err
		}
		return nil, SourceBundled, 0, err
	}

	bundle, parseErr := parseBundle(body)
	if parseErr != nil {
		return nil, SourceBundled, 0, fmt.Errorf("remote registry parse error: %w", parseErr)
	}
	// Write-through to cache; best effort, caller doesn't fail if the disk is read-only.
	_ = writeCache(cachePath, body)
	return &bundle, SourceNetwork, 0, nil
}

// fetchOverNetwork performs the bounded GET and returns the body bytes.
// Uses a transport that caps dial + TLS handshake so pathological networks
// can't stretch the budget past fetchTimeout.
func fetchOverNetwork(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "stackwright/0.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	// Bound body size — a runaway registry file shouldn't blow up memory.
	const maxRegistryBytes = 4 * 1024 * 1024
	return io.ReadAll(io.LimitReader(resp.Body, maxRegistryBytes))
}

// readFreshCache returns (bundle, age, true) only if the cache exists and is
// newer than cacheMaxAge. An older cache is treated as "needs refresh."
func readFreshCache(path string) (Bundle, time.Duration, bool) {
	bundle, age, ok := readAnyCache(path)
	if !ok {
		return Bundle{}, 0, false
	}
	if age > cacheMaxAge {
		return Bundle{}, age, false
	}
	return bundle, age, true
}

// readAnyCache returns (bundle, age, true) for any valid cache file,
// regardless of age. Used as a network-failure fallback.
func readAnyCache(path string) (Bundle, time.Duration, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return Bundle{}, 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Bundle{}, 0, false
	}
	bundle, err := parseBundle(data)
	if err != nil {
		return Bundle{}, 0, false
	}
	return bundle, time.Since(info.ModTime()), true
}

// writeCache persists the fetched registry alongside XDG config.
// Best-effort; failure to write doesn't fail the Load.
func writeCache(path string, data []byte) error {
	if err := config.EnsureDir(config.ConfigDir()); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// parseBundle accepts either JSON or YAML input. yaml.v3 handles both because
// JSON is a valid subset of YAML 1.2.
func parseBundle(data []byte) (Bundle, error) {
	var b Bundle
	if err := yaml.Unmarshal(data, &b); err != nil {
		return Bundle{}, err
	}
	if b.Entries == nil {
		return Bundle{}, errors.New("registry has no entries")
	}
	return b, nil
}

// mergeByslug returns base with overlay entries applied on top.
// Matching slugs: overlay wins. New slugs: appended.
func mergeByslug(base, overlay Bundle) Bundle {
	index := make(map[string]int, len(base.Entries))
	for i, e := range base.Entries {
		index[e.Slug] = i
	}
	for _, e := range overlay.Entries {
		if i, ok := index[e.Slug]; ok {
			base.Entries[i] = e
		} else {
			base.Entries = append(base.Entries, e)
			index[e.Slug] = len(base.Entries) - 1
		}
	}
	if overlay.Version != "" {
		base.Version = overlay.Version
	}
	return base
}
