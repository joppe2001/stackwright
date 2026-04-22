package phase_design

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// unknownModal is the overlay form used to capture a new technology from the
// user on the fly. Triggered from the sub-list with 'a'. On save, the entry
// is written to registry.local.yaml via registry.AppendLocal and appended to
// the in-memory bundle so it appears immediately in the layer navigator.
type unknownModal struct {
	visible bool

	// Pre-seeded layer — drives the default category and what we select the
	// newly-added entry into once saved.
	forLayer tui.Layer

	// Focus is 0..len(fields)+1 — the "+1" row is the Save button.
	cursor int

	nameInput       textinput.Model
	slugInput       textinput.Model
	descInput       textinput.Model
	installInput    textinput.Model
	docsInput       textinput.Model
	logoInput       textinput.Model
	signupInput     textinput.Model
	authCmdInput    textinput.Model
	authVerifyInput textinput.Model

	// Radio-style selections.
	accountRequired bool
	category        registry.Category

	saveError string
}

// newUnknownModal returns a default modal. Callers should call .Open() before
// showing it so the text inputs are seeded for the target layer.
func newUnknownModal() unknownModal {
	mk := func(ph string) textinput.Model {
		t := textinput.New()
		t.Placeholder = ph
		t.Prompt = ""
		t.CharLimit = 200
		t.Width = 44
		return t
	}
	return unknownModal{
		nameInput:       mk("Trigger.dev"),
		slugInput:       mk("triggerdev"),
		descInput:       mk("short one-line description"),
		installInput:    mk("npm install -g @trigger.dev/cli"),
		docsInput:       mk("https://trigger.dev"),
		logoInput:       mk("https://… (optional — monogram used otherwise)"),
		signupInput:     mk("https://cloud.trigger.dev/sign-up"),
		authCmdInput:    mk("npx @trigger.dev/cli@latest login"),
		authVerifyInput: mk("npx @trigger.dev/cli@latest whoami"),
	}
}

// Open initializes the modal for a target layer, pre-seeds the category,
// clears old values, and focuses the first input.
func (u *unknownModal) Open(l tui.Layer) {
	u.visible = true
	u.forLayer = l
	u.cursor = 0
	u.saveError = ""
	// Default category comes from the layer the user was browsing, falling
	// back to "service" for the catch-all.
	if cat, ok := tui.LayerCategory(l); ok {
		u.category = cat
	} else {
		u.category = registry.CategoryService
	}
	// Blur everything, then focus the first field.
	u.focus(0)
}

// Close hides the modal and resets focus/state for the next open.
func (u *unknownModal) Close() {
	u.visible = false
	u.blurAll()
}

// fields returns pointers to each input in draw order. Keeping this list in
// one place means tab-navigation and the save path always agree on the shape.
func (u *unknownModal) fields() []*textinput.Model {
	return []*textinput.Model{
		&u.nameInput,
		&u.slugInput,
		&u.descInput,
		&u.installInput,
		&u.docsInput,
		&u.logoInput,
		&u.signupInput,
		&u.authCmdInput,
		&u.authVerifyInput,
	}
}

// cursorCount is the number of focusable rows: text inputs + 2 radios + 1 save button.
func (u unknownModal) cursorCount() int { return len(u.fields()) + 3 }

func (u *unknownModal) focus(idx int) {
	u.blurAll()
	u.cursor = idx
	if idx < len(u.fields()) {
		u.fields()[idx].Focus()
	}
}

func (u *unknownModal) blurAll() {
	for _, f := range u.fields() {
		f.Blur()
	}
}

// Update handles keystrokes when the modal is visible.
// Returns (savedEntry, saved, cancelled, cmd).
//
// `saved` is true when the user successfully accepted the form;
// `cancelled` when esc was pressed. Both cases close the modal.
func (u *unknownModal) Update(msg tea.Msg) (registry.Entry, bool, bool, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return registry.Entry{}, false, false, nil
	}

	switch km.Type {
	case tea.KeyEsc:
		u.Close()
		return registry.Entry{}, false, true, nil
	case tea.KeyTab, tea.KeyDown:
		u.focus((u.cursor + 1) % u.cursorCount())
		return registry.Entry{}, false, false, nil
	case tea.KeyShiftTab, tea.KeyUp:
		u.focus((u.cursor - 1 + u.cursorCount()) % u.cursorCount())
		return registry.Entry{}, false, false, nil
	}

	fieldCount := len(u.fields())

	// Radio rows (category, accountRequired) — handled via left/right/space.
	switch u.cursor {
	case fieldCount: // category radio
		if km.Type == tea.KeyLeft {
			u.category = prevCategory(u.category)
			return registry.Entry{}, false, false, nil
		}
		if km.Type == tea.KeyRight || km.Type == tea.KeySpace {
			u.category = nextCategory(u.category)
			return registry.Entry{}, false, false, nil
		}
	case fieldCount + 1: // accountRequired toggle
		if km.Type == tea.KeySpace || km.Type == tea.KeyLeft || km.Type == tea.KeyRight {
			u.accountRequired = !u.accountRequired
			return registry.Entry{}, false, false, nil
		}
	case fieldCount + 2: // save button
		if km.Type == tea.KeyEnter {
			entry, err := u.buildEntry()
			if err != nil {
				u.saveError = err.Error()
				return registry.Entry{}, false, false, nil
			}
			if err := registry.AppendLocal(entry); err != nil {
				u.saveError = "could not save: " + err.Error()
				return registry.Entry{}, false, false, nil
			}
			u.Close()
			return entry, true, false, nil
		}
	}

	// Text inputs: forward to the focused input.
	if u.cursor < fieldCount {
		var cmd tea.Cmd
		*u.fields()[u.cursor], cmd = u.fields()[u.cursor].Update(msg)
		// Auto-fill slug from the name on the fly (only if the user hasn't
		// touched the slug field explicitly).
		if u.cursor == 0 && u.slugInput.Value() == "" {
			u.slugInput.SetValue(slugify(u.nameInput.Value()))
		}
		return registry.Entry{}, false, false, cmd
	}
	return registry.Entry{}, false, false, nil
}

// View renders the modal overlaid on the full design-phase canvas.
// The caller composites this on top of the background using lipgloss.Place.
func (u unknownModal) View(width, height int) string {
	var b strings.Builder
	header := theme.Accent.Render("ADD TECHNOLOGY") +
		theme.Dim.Render(fmt.Sprintf("  —  saved to %s", "~/.config/stackwright/registry.local.yaml"))
	b.WriteString(header)
	b.WriteString("\n\n")

	fieldCount := len(u.fields())
	labels := []string{
		"Name",
		"Slug",
		"Description",
		"Install command",
		"Docs / repo URL",
		"Logo URL",
		"Signup URL",
		"Auth command",
		"Auth verify cmd",
	}
	for i, f := range u.fields() {
		marker := "  "
		if u.cursor == i {
			marker = "▸ "
		}
		b.WriteString(marker)
		b.WriteString(fmt.Sprintf("%-17s", labels[i]))
		b.WriteString(f.View())
		b.WriteString("\n")
	}

	// Category radio.
	b.WriteString("\n")
	catMarker := "  "
	if u.cursor == fieldCount {
		catMarker = "▸ "
	}
	b.WriteString(catMarker)
	b.WriteString(fmt.Sprintf("%-17s[ %s ]  ", "Category", u.category))
	b.WriteString(theme.Dim.Render("← → to change"))
	b.WriteString("\n")

	// Account required toggle.
	accMarker := "  "
	if u.cursor == fieldCount+1 {
		accMarker = "▸ "
	}
	yn := "no"
	if u.accountRequired {
		yn = "yes"
	}
	b.WriteString(accMarker)
	b.WriteString(fmt.Sprintf("%-17s[ %s ]  ", "Account required", yn))
	b.WriteString(theme.Dim.Render("space to toggle"))
	b.WriteString("\n")

	// Save row.
	b.WriteString("\n")
	saveMarker := "  "
	if u.cursor == fieldCount+2 {
		saveMarker = "▸ "
	}
	b.WriteString(saveMarker)
	b.WriteString(theme.Good.Render("[ Save ]"))
	b.WriteString("  ")
	b.WriteString(theme.Dim.Render("enter to save   esc to cancel"))

	if u.saveError != "" {
		b.WriteString("\n\n")
		b.WriteString(theme.Accent.Render("error: ") + u.saveError)
	}

	// Box the whole thing up inside a centered modal frame.
	box := theme.Modal.Render(b.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// buildEntry assembles a registry.Entry from the current form values,
// applying validation and sensible defaults (diagram color, verified flag).
func (u unknownModal) buildEntry() (registry.Entry, error) {
	name := strings.TrimSpace(u.nameInput.Value())
	if name == "" {
		return registry.Entry{}, fmt.Errorf("name is required")
	}
	slug := strings.TrimSpace(u.slugInput.Value())
	if slug == "" {
		slug = slugify(name)
	}

	e := registry.Entry{
		Name:         name,
		Slug:         slug,
		Category:     u.category,
		Description:  strings.TrimSpace(u.descInput.Value()),
		DiagramColor: "#7c6ff7", // theme accent purple; user can edit the YAML to change
		LogoURL:      strings.TrimSpace(u.logoInput.Value()),
		DocsURL:      strings.TrimSpace(u.docsInput.Value()),
		AddedBy:      "local",
		Verified:     false,
	}

	if install := strings.TrimSpace(u.installInput.Value()); install != "" {
		e.CLI = &registry.CLI{
			Binary:     inferBinary(install),
			VersionCmd: inferBinary(install) + " --version",
			Install: &registry.Install{
				Macos:   install,
				Linux:   install,
				Windows: install,
			},
		}
	}

	if u.accountRequired {
		e.Account = &registry.Account{
			Required:  true,
			SignupURL: strings.TrimSpace(u.signupInput.Value()),
		}
	}

	if authCmd := strings.TrimSpace(u.authCmdInput.Value()); authCmd != "" {
		e.Auth = &registry.Auth{
			Required:  true,
			Cmd:       authCmd,
			VerifyCmd: strings.TrimSpace(u.authVerifyInput.Value()),
		}
	}

	return e, nil
}

// slugify lowercases a name and strips non-[a-z0-9] runes, joining with hyphens.
func slugify(s string) string {
	var b strings.Builder
	last := rune(0)
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			last = r
		default:
			if last != '-' && last != 0 {
				b.WriteRune('-')
				last = '-'
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// inferBinary picks a plausible binary name from an install command so we can
// populate CLI.Binary without a dedicated form field. Heuristic only — user
// can fix by editing registry.local.yaml.
func inferBinary(install string) string {
	fields := strings.Fields(install)
	for _, f := range fields {
		switch f {
		case "brew", "apt-get", "apt", "sudo", "winget", "scoop", "npm", "install", "-g", "-y", "-D", "-fsSL", "curl", "|", "sh":
			continue
		}
		// First non-boilerplate token wins.
		return strings.TrimSuffix(f, ":")
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// allCategories is the display order used by the category radio.
var allCategories = []registry.Category{
	registry.CategoryFrontend,
	registry.CategoryBackend,
	registry.CategoryDatabase,
	registry.CategoryCache,
	registry.CategoryAuth,
	registry.CategoryPayments,
	registry.CategoryInfra,
	registry.CategoryCICD,
	registry.CategoryService,
}

func prevCategory(c registry.Category) registry.Category {
	for i, v := range allCategories {
		if v == c {
			if i == 0 {
				return allCategories[len(allCategories)-1]
			}
			return allCategories[i-1]
		}
	}
	return allCategories[0]
}

func nextCategory(c registry.Category) registry.Category {
	for i, v := range allCategories {
		if v == c {
			return allCategories[(i+1)%len(allCategories)]
		}
	}
	return allCategories[0]
}
