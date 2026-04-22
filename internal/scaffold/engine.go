// Package scaffold generates the final project from templates + the
// confirmed stack. It also emits stack.yaml, SETUP.md, and an architecture
// PNG alongside the project directory.
//
// Templates live under /templates/<category>/<slug>/ in the source tree
// and are compiled into the binary via embed.FS. Each template directory
// contains an optional manifest.yaml listing the files to copy with an
// output path and merge strategy; everything else is rendered through
// Go text/template with the user's stack values.
package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
)

// TemplatesFS embeds the templates/ tree shipped with the source.
// Using "all:templates" so hidden files (starting with . or _) are embedded too.
//
//go:embed all:templates
var TemplatesFS embed.FS

// TemplateVars are the values available inside .tmpl files.
// Keep the field set small and obvious — template authors shouldn't need
// to consult Go source to know what's available.
type TemplateVars struct {
	AppName         string
	Stack           map[string]string // layer name → slug
	AuthEnabled     bool
	PaymentsEnabled bool
	DBName          string
	GoModulePath    string
}

// buildVars pulls TemplateVars from the user's Stack.
// GoModulePath defaults to <appname>.local so generated go.mod compiles
// locally without GitHub; users can rewrite it after init.
func buildVars(stack tui.Stack) TemplateVars {
	stackMap := map[string]string{}
	for _, l := range tui.AllLayers {
		if slug := stack.Slug(l); slug != "" {
			stackMap[string(l)] = slug
		}
	}
	return TemplateVars{
		AppName:         stack.AppName,
		Stack:           stackMap,
		AuthEnabled:     stack.Slug(tui.LayerAuth) != "",
		PaymentsEnabled: stack.Slug(tui.LayerPayments) != "",
		DBName:          strings.ReplaceAll(stack.AppName, "-", "_"),
		GoModulePath:    fmt.Sprintf("local/%s", slugifyModule(stack.AppName)),
	}
}

// Manifest describes how to copy one template tree into the output.
// Omitted fields get sensible defaults.
type Manifest struct {
	Files []ManifestFile `yaml:"files"`
}

// ManifestFile is one src→dst mapping. Merge strategy applies when the
// destination path already exists (from an earlier template for the same project).
//
//   - replace: overwrite with the new content (default)
//   - append:  append the new content to existing
//   - merge-yaml: deep-merge as YAML (both files must parse as YAML)
type ManifestFile struct {
	Src   string `yaml:"src"`
	Dst   string `yaml:"dst"`
	Merge string `yaml:"merge,omitempty"`
}

// FileResult is one rendered output file, produced by Generate.
// The stream format (via a channel) lets the TUI show per-file progress.
type FileResult struct {
	Path  string
	Size  int
	Err   error
	Total int // total files expected (only set on the first result)
}

// Plan describes everything Generate would do for a given stack, without
// writing anything. Used by the scaffold TUI to show the upcoming work.
type Plan struct {
	OutDir string
	Files  []PlannedFile // project files (including stack.yaml, SETUP.md)
	PNG    string        // architecture PNG path (sibling of OutDir)
	Errors []error
}

// PlannedFile is one file the scaffold will produce.
// AbsPath is the final on-disk location; Source is either a template path
// or a synthetic marker like "<stack.yaml>" for generated specs.
type PlannedFile struct {
	AbsPath string
	Source  string
	Merge   string
}

// BuildPlan walks the embedded templates for every selected entry and emits
// the list of files that will be generated — including duplicates, because
// merging (append / merge-yaml) happens per-file-write in Generate. Keeping
// every contributor in order lets later templates append/merge into earlier ones.
func BuildPlan(outDir string, bundle registry.Bundle, stack tui.Stack) Plan {
	plan := Plan{OutDir: outDir, PNG: outDir + "-architecture.png"}

	for _, layer := range tui.AllLayers {
		slug := stack.Slug(layer)
		if slug == "" {
			continue
		}
		e, ok := bundle.BySlug(slug)
		if !ok {
			continue
		}
		entries, err := templateFilesFor(e)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			plan.Errors = append(plan.Errors, fmt.Errorf("%s: %w", e.Slug, err))
			continue
		}
		for _, mf := range entries {
			dst := filepath.Join(outDir, mf.Dst)
			plan.Files = append(plan.Files, PlannedFile{AbsPath: dst, Source: mf.Src, Merge: mf.Merge})
		}
	}

	// Always add the generated spec files at the end.
	plan.Files = append(plan.Files,
		PlannedFile{AbsPath: filepath.Join(outDir, "stack.yaml"), Source: "<generated stack.yaml>", Merge: "replace"},
		PlannedFile{AbsPath: filepath.Join(outDir, "SETUP.md"), Source: "<generated SETUP.md>", Merge: "replace"},
	)
	return plan
}

// Generate writes the project to outDir. Emits a FileResult for every file
// (success or error) on the returned channel, then closes the channel.
//
// Callers typically launch Generate in a goroutine and consume the channel
// from the TUI via tea.Cmd, so progress is visible as each file lands.
func Generate(outDir string, bundle registry.Bundle, stack tui.Stack) <-chan FileResult {
	results := make(chan FileResult, 16)
	go func() {
		defer close(results)
		plan := BuildPlan(outDir, bundle, stack)
		total := len(plan.Files)
		vars := buildVars(stack)

		// Fire a sentinel first so the UI knows the total.
		results <- FileResult{Total: total}

		if err := os.MkdirAll(outDir, 0o755); err != nil {
			results <- FileResult{Err: fmt.Errorf("mkdir %s: %w", outDir, err)}
			return
		}

		existing := map[string][]byte{} // dst → bytes-written-so-far (for append/merge)

		for _, pf := range plan.Files {
			var content []byte
			var err error
			switch pf.Source {
			case "<generated stack.yaml>":
				content, err = renderStackYAML(stack, bundle)
			case "<generated SETUP.md>":
				content, err = renderSetupMD(stack, bundle)
			default:
				content, err = renderTemplate(pf.Source, vars)
			}
			if err != nil {
				results <- FileResult{Path: pf.AbsPath, Err: err}
				continue
			}

			final, merr := applyMerge(existing[pf.AbsPath], content, pf.Merge)
			if merr != nil {
				results <- FileResult{Path: pf.AbsPath, Err: merr}
				continue
			}
			existing[pf.AbsPath] = final

			if err := writeFileMkdir(pf.AbsPath, final); err != nil {
				results <- FileResult{Path: pf.AbsPath, Err: err}
				continue
			}
			results <- FileResult{Path: pf.AbsPath, Size: len(final)}
		}
	}()
	return results
}

// templateFilesFor returns the manifest-resolved file list for one entry's
// template directory. Missing manifest is not an error — we fall back to a
// "copy every file under the template root, preserving paths" default.
func templateFilesFor(e registry.Entry) ([]ManifestFile, error) {
	root := filepath.Join("templates", string(e.Category), e.Slug)
	// Check if the template dir exists in the embed.
	if _, err := fs.Stat(TemplatesFS, root); err != nil {
		return nil, err
	}

	// Manifest first (if present).
	manifestPath := filepath.Join(root, "manifest.yaml")
	if data, err := fs.ReadFile(TemplatesFS, manifestPath); err == nil {
		var m Manifest
		if yerr := yaml.Unmarshal(data, &m); yerr != nil {
			return nil, fmt.Errorf("parse %s: %w", manifestPath, yerr)
		}
		// Normalize Src paths to be relative to the template root.
		for i := range m.Files {
			m.Files[i].Src = filepath.Join(root, m.Files[i].Src)
			if m.Files[i].Merge == "" {
				m.Files[i].Merge = "replace"
			}
		}
		return m.Files, nil
	}

	// No manifest: walk the template dir and copy everything, preserving
	// paths (minus the .tmpl extension if present).
	var out []ManifestFile
	err := fs.WalkDir(TemplatesFS, root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || strings.HasSuffix(p, "/manifest.yaml") {
			return nil
		}
		rel := strings.TrimPrefix(p, root+"/")
		dst := strings.TrimSuffix(rel, ".tmpl")
		out = append(out, ManifestFile{Src: p, Dst: dst, Merge: "replace"})
		return nil
	})
	return out, err
}

// renderTemplate reads a template file from the embedded FS and applies vars.
// Non-.tmpl files are copied verbatim (no template processing) so binary
// assets pass through untouched.
func renderTemplate(srcPath string, vars TemplateVars) ([]byte, error) {
	data, err := fs.ReadFile(TemplatesFS, srcPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcPath, err)
	}
	if !strings.HasSuffix(srcPath, ".tmpl") {
		return data, nil
	}
	t, err := template.New(path.Base(srcPath)).Option("missingkey=zero").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", srcPath, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("execute %s: %w", srcPath, err)
	}
	return buf.Bytes(), nil
}

// applyMerge combines existing and incoming bytes per strategy.
// An empty `existing` means this is the first template to write here;
// the strategy is effectively irrelevant and we just return `incoming`.
func applyMerge(existing, incoming []byte, strategy string) ([]byte, error) {
	if len(existing) == 0 {
		return incoming, nil
	}
	switch strategy {
	case "", "replace":
		return incoming, nil
	case "append":
		if bytes.HasSuffix(existing, []byte("\n")) {
			return append(existing, incoming...), nil
		}
		return append(append(existing, '\n'), incoming...), nil
	case "merge-yaml":
		return mergeYAML(existing, incoming)
	default:
		return nil, fmt.Errorf("unknown merge strategy %q", strategy)
	}
}

// mergeYAML deep-merges two YAML documents (map-on-map). Lists are concatenated.
// A scalar collision wins for the incoming value — matches what most template
// authors mean when they say "merge."
func mergeYAML(a, b []byte) ([]byte, error) {
	var na, nb yaml.Node
	if err := yaml.Unmarshal(a, &na); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &nb); err != nil {
		return nil, err
	}
	merged := mergeYAMLNodes(&na, &nb)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(merged); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// mergeYAMLNodes walks two YAML nodes and returns a merged node.
// Document-level wrappers are unwrapped first.
func mergeYAMLNodes(a, b *yaml.Node) *yaml.Node {
	if a.Kind == yaml.DocumentNode {
		if len(a.Content) > 0 {
			a = a.Content[0]
		}
	}
	if b.Kind == yaml.DocumentNode {
		if len(b.Content) > 0 {
			b = b.Content[0]
		}
	}
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.Kind == yaml.MappingNode && b.Kind == yaml.MappingNode {
		keys := map[string]int{}
		for i := 0; i < len(a.Content); i += 2 {
			keys[a.Content[i].Value] = i
		}
		for i := 0; i < len(b.Content); i += 2 {
			k, v := b.Content[i], b.Content[i+1]
			if idx, ok := keys[k.Value]; ok {
				a.Content[idx+1] = mergeYAMLNodes(a.Content[idx+1], v)
			} else {
				a.Content = append(a.Content, k, v)
			}
		}
		return a
	}
	if a.Kind == yaml.SequenceNode && b.Kind == yaml.SequenceNode {
		a.Content = append(a.Content, b.Content...)
		return a
	}
	return b // scalar / mismatched types: incoming wins
}

// writeFileMkdir creates parent directories as needed before writing.
func writeFileMkdir(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// slugifyModule turns "My SaaS" into "my-saas" for Go module paths.
func slugifyModule(s string) string {
	var b strings.Builder
	last := rune(0)
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			last = r
		case last != '-' && last != 0:
			b.WriteRune('-')
			last = '-'
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "app"
	}
	return out
}
