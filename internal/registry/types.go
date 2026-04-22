// Package registry models the technology catalog that drives every phase of
// stackwright: the design diagram, the setup wizard, and the scaffold engine.
//
// The schema here must stay a pure data model — NO technology-specific logic
// lives in Go source. Everything (install commands, auth flows, compatible
// pairs, diagram colors) is data read from Entry values at runtime.
package registry

// Category identifies the layer an entry belongs to. The setup wizard also
// uses this to compute a dependency-ordered install sequence.
type Category string

const (
	CategoryFrontend Category = "frontend"
	CategoryBackend  Category = "backend"
	CategoryDatabase Category = "database"
	CategoryCache    Category = "cache"
	CategoryAuth     Category = "auth"
	CategoryPayments Category = "payments"
	CategoryInfra    Category = "infra"
	CategoryCICD     Category = "cicd"
	CategoryService  Category = "service"
)

// Entry is one technology in the catalog. Populated from YAML/JSON in the
// bundled registry, the synced GitHub registry, or the user's local additions.
//
// Field tags use yaml (not json) because all three sources are YAML-compatible
// and yaml.v3 handles both JSON and YAML input.
type Entry struct {
	Name     string   `yaml:"name"`
	Slug     string   `yaml:"slug"`
	Category Category `yaml:"category"`

	// Design phase.
	Description    string   `yaml:"description"`
	CompatibleWith []string `yaml:"compatible_with,omitempty"`
	LogoURL        string   `yaml:"logo_url,omitempty"`
	DiagramColor   string   `yaml:"diagram_color"`

	// Setup phase.
	CLI     *CLI     `yaml:"cli,omitempty"`
	Account *Account `yaml:"account,omitempty"`
	Auth    *Auth    `yaml:"auth,omitempty"`

	// Scaffold phase.
	BoilerplateSnippet string   `yaml:"boilerplate_snippet,omitempty"`
	SetupSteps         []string `yaml:"setup_steps,omitempty"`
	DocsURL            string   `yaml:"docs_url,omitempty"`

	AddedBy  string `yaml:"added_by,omitempty"`
	Verified bool   `yaml:"verified,omitempty"`
}

// CLI describes how to check for and install the technology's command-line tool.
// A nil CLI means the tech has no CLI (e.g., Stripe as a pure SDK).
type CLI struct {
	Binary     string   `yaml:"binary"`
	VersionCmd string   `yaml:"version_cmd"`
	Install    *Install `yaml:"install,omitempty"`
}

// Install holds per-platform install commands. The setup wizard picks one by
// runtime.GOOS (darwin→Macos, linux→Linux, windows→Windows).
type Install struct {
	Macos   string `yaml:"macos,omitempty"`
	Linux   string `yaml:"linux,omitempty"`
	Windows string `yaml:"windows,omitempty"`
}

// Account describes account-signup requirements. Absent (nil) or
// Required=false means the setup wizard skips the account prompt.
type Account struct {
	Required  bool   `yaml:"required"`
	SignupURL string `yaml:"signup_url,omitempty"`
	Note      string `yaml:"note,omitempty"`
}

// Auth describes the CLI auth flow. Absent (nil) or Required=false means
// the setup wizard skips auth and verify steps for this entry.
type Auth struct {
	Required             bool   `yaml:"required"`
	Cmd                  string `yaml:"cmd,omitempty"`
	VerifyCmd            string `yaml:"verify_cmd,omitempty"`
	VerifySuccessPattern string `yaml:"verify_success_pattern,omitempty"`
	OpensBrowser         bool   `yaml:"opens_browser,omitempty"`
	Note                 string `yaml:"note,omitempty"`
}

// Bundle is the top-level structure of the registry file itself
// (both bundled JSON and user registry.local.yaml).
type Bundle struct {
	Version string  `yaml:"version" json:"version"`
	Entries []Entry `yaml:"entries" json:"entries"`
}
