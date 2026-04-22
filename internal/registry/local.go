package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/joppe2001/stackwright/internal/config"
	"gopkg.in/yaml.v3"
)

// ReadLocal loads the user's local registry.local.yaml. A missing file is not
// an error — it simply means the user hasn't added any entries yet, and we
// return (nil, nil). Any parse error IS returned so the user learns their
// edits broke the file.
func ReadLocal() (*Bundle, error) {
	path := config.RegistryLocalPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var b Bundle
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &b, nil
}

// AppendLocal adds one entry to registry.local.yaml, creating the file if it
// doesn't exist yet. If an entry with the same slug already exists, it is
// replaced (consistent with mergeByslug's overlay semantics).
//
// Uses a read-modify-write pattern — YAML diffs produced by hand-editing
// users are preserved when we only touch the entry they added.
func AppendLocal(e Entry) error {
	if err := config.EnsureDir(config.ConfigDir()); err != nil {
		return err
	}

	path := config.RegistryLocalPath()
	bundle := Bundle{Version: "1.0"}

	if existing, err := ReadLocal(); err != nil {
		return err
	} else if existing != nil {
		bundle = *existing
	}

	replaced := false
	for i := range bundle.Entries {
		if bundle.Entries[i].Slug == e.Slug {
			bundle.Entries[i] = e
			replaced = true
			break
		}
	}
	if !replaced {
		bundle.Entries = append(bundle.Entries, e)
	}

	data, err := yaml.Marshal(bundle)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
