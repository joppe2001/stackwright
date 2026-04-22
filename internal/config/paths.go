// Package config computes the XDG-compliant config and cache paths stackwright
// uses at runtime. Everything the tool persists between runs — registry cache,
// local user-added entries, cached logo PNGs — lives under these paths.
package config

import (
	"os"
	"path/filepath"
)

const appDirName = "stackwright"

// ConfigDir returns $XDG_CONFIG_HOME/stackwright (or the platform default).
// Directory is not created here; callers that write to it should MkdirAll first.
func ConfigDir() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, appDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "."+appDirName)
	}
	return filepath.Join(home, ".config", appDirName)
}

// CacheDir returns $XDG_CACHE_HOME/stackwright (or the platform default).
func CacheDir() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, appDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "."+appDirName, "cache")
	}
	return filepath.Join(home, ".cache", appDirName)
}

// RegistryCachePath is the on-disk location of the synced registry.
func RegistryCachePath() string { return filepath.Join(ConfigDir(), "registry.cache.yaml") }

// RegistryLocalPath is where user-added registry entries live.
func RegistryLocalPath() string { return filepath.Join(ConfigDir(), "registry.local.yaml") }

// LogoCacheDir is where fetched tech logos are cached.
func LogoCacheDir() string { return filepath.Join(ConfigDir(), "logos") }

// EnsureDir creates a directory (and parents) if it does not exist. Idempotent.
func EnsureDir(path string) error { return os.MkdirAll(path, 0o755) }
