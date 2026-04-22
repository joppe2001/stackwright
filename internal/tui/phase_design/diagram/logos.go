package diagram

import (
	"context"
	"crypto/tls"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/image/draw"

	"github.com/joppe2001/stackwright/internal/config"
)

// Logo fetching for visual mode.
//
// Design choices:
//   - All logos cached to disk (XDG cache) so first-launch cost is amortized.
//   - Fetch happens lazily on first diagram render that actually needs a logo.
//   - Failure of any kind (404, timeout, decode error, non-PNG) falls back
//     to the monogram renderer silently — logos are a nice-to-have, not a
//     blocker for visual mode.
//   - In-memory cache de-dupes lookups for the same URL within a session.
//
// The logo renderer is invoked from drawPNGNode only when a logo is cached
// and decoded successfully, so there's no performance hit when logos are
// missing — the monogram path is the default.

const (
	logoPixelSize = 32 // square size logos are resized to before placement in the node card
	logoHTTPTimeout = 4 * time.Second
	maxLogoBytes    = 512 * 1024 // 512 KiB; real logos are ~10-50 KiB
)

// logoCache is the in-memory cache of successfully-fetched, decoded logos.
// Key is the logo URL. Value is the resized 32x32 RGBA or nil if the URL
// has been tried and failed (negative cache — don't retry in this session).
var (
	logoCacheMu sync.RWMutex
	logoCache   = map[string]*image.RGBA{}
	logoTried   = map[string]bool{} // negative cache of failed URLs
)

// loadLogo returns a resized logo image for the given URL, or nil if not
// available. Checks:
//  1. Negative cache — if we already tried and failed this URL, return nil.
//  2. In-memory cache — if we have the resized image, return it.
//  3. On-disk cache (~/.config/stackwright/logos/<hash>.png) — load, resize,
//     cache in memory, return.
//  4. Network fetch — GET the URL with a short timeout, save the raw bytes
//     to disk cache, then fall through to step 3.
//
// Thread-safe. Safe to call from the render path on every frame because the
// happy path hits the in-memory map.
func loadLogo(url string) *image.RGBA {
	if url == "" {
		return nil
	}

	// Fast path: positive cache hit.
	logoCacheMu.RLock()
	if img, ok := logoCache[url]; ok {
		logoCacheMu.RUnlock()
		return img
	}
	if logoTried[url] {
		logoCacheMu.RUnlock()
		return nil
	}
	logoCacheMu.RUnlock()

	// Slow path: attempt to load (may hit disk or network).
	img := doLoadLogo(url)

	logoCacheMu.Lock()
	if img == nil {
		logoTried[url] = true
	} else {
		logoCache[url] = img
	}
	logoCacheMu.Unlock()

	return img
}

// doLoadLogo is the un-cached loader. Separated so the caching logic above
// stays simple.
func doLoadLogo(url string) *image.RGBA {
	path := diskCachePath(url)

	// Try on-disk cache first.
	if img, ok := readLogoFromDisk(path); ok {
		return img
	}

	// Fetch fresh.
	if err := downloadLogo(url, path); err != nil {
		// Silent failure — visual mode falls back to monograms.
		return nil
	}

	// Read back what we just wrote.
	if img, ok := readLogoFromDisk(path); ok {
		return img
	}
	return nil
}

// diskCachePath computes the on-disk cache location for a logo URL.
// Uses the URL's basename rather than a hash so the cache is human-browsable
// — a user peeking at ~/.config/stackwright/logos/ sees "flyio.png" rather
// than opaque hashes.
func diskCachePath(url string) string {
	base := filepath.Base(url)
	if base == "" || base == "." || base == "/" {
		base = "unknown.png"
	}
	// Defensively strip any query string.
	if i := lastIndexByte(base, '?'); i >= 0 {
		base = base[:i]
	}
	return filepath.Join(config.LogoCacheDir(), base)
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// readLogoFromDisk loads the PNG at path and resizes it to logoPixelSize.
// Returns (img, true) on success; (nil, false) on any error.
func readLogoFromDisk(path string) (*image.RGBA, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	img, err := png.Decode(io.LimitReader(f, maxLogoBytes))
	if err != nil {
		return nil, false
	}
	return resizeToRGBA(img, logoPixelSize, logoPixelSize), true
}

// downloadLogo fetches the URL and writes the raw bytes to path.
// Enforces a 4s total budget and a 512 KiB body cap so a rogue server can't
// stall or balloon memory. Creates the cache directory as needed.
func downloadLogo(url, path string) error {
	if err := config.EnsureDir(config.LogoCacheDir()); err != nil {
		return err
	}

	client := &http.Client{
		Timeout: logoHTTPTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   logoHTTPTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   logoHTTPTimeout,
			ResponseHeaderTimeout: logoHTTPTimeout,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), logoHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "stackwright/logo-fetch")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".logo-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Cap body size so a pathological CDN can't hand us a 50 MB "logo".
	_, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, maxLogoBytes))
	_ = tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	// Atomic replace so a concurrent reader never sees a half-written file.
	return os.Rename(tmpPath, path)
}

// resizeToRGBA scales src to an RGBA image of (w, h) pixels using bilinear
// filtering. Returns an owned *image.RGBA that the caller may read freely.
func resizeToRGBA(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
