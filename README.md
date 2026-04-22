# stackwright

Terminal-native, interactive full-stack project scaffolder.

stackwright composes a stack layer-by-layer in a split-pane TUI with a live
architecture diagram on the right, runs every selected technology's CLI
install + auth flows to verified success, and scaffolds a complete project
(`stack.yaml`, `SETUP.md`, boilerplate, architecture PNG) at the end.

## Install

```
npm install -g stackwright
```

The npm package ships a tiny Node shim; at postinstall it downloads the
prebuilt Go binary matching your platform (macOS arm64/amd64, Linux
amd64/arm64, Windows amd64) and prints a terminal capability report.

## Usage

```
stackwright                # launch the interactive TUI
stackwright --detect       # print a terminal capability report and exit
stackwright --no-kitty     # force standard ANSI/Unicode mode
stackwright --offline      # skip registry sync, use bundled + local only
stackwright registry list  # dump the merged registry
stackwright registry share <slug>   # open a GitHub issue to upstream a local entry
```

## How it works

Three phases, state-machine-driven:

1. **Design** — split-pane: layer navigator on the left, live vertical
   architecture diagram on the right. Pick a technology per layer; the
   diagram updates as you go. Press `a` in a sub-list to add a tech that
   isn't in the registry — it's saved locally and usable immediately.
   Press `g` to advance.
2. **Setup** — for every technology, in dependency order (infra → database
   → cache → auth → payments → backend → frontend → cicd → services):
   checks if its CLI is installed, installs it if not, prompts you to
   create an account if required, runs the auth flow, and verifies
   success. Each CLI runs under a PTY so browser-based auth works.
3. **Scaffold** — generates the project: boilerplate from templates for
   every selected tech, merged per the template manifests (replace /
   append / merge-yaml); writes `stack.yaml`, `SETUP.md`, and a
   `<name>-architecture.png` sibling image.

## Rendering modes

Detected at launch — Ghostty / Kitty get the **visual** renderer (pixel
PNG architecture diagram over the Kitty Graphics Protocol). Other
terminals get the **standard** renderer (ANSI/Unicode box-drawing and
bezier-sampled connection traces). Both modes are fully featured.

Force standard mode with `--no-kitty`.

## Registry

Every technology's metadata — install commands, account requirements,
auth flow, compatible peers, diagram color — lives in a community
registry:

- **Bundled**: compiled into the binary. Always available.
- **Synced**: fetched from [joppe2001/stackwright-registry][registry] on
  launch with a 24h local cache and a 3s network timeout.
- **Local**: `~/.config/stackwright/registry.local.yaml`. User-added
  entries overlay everything else by slug.

To contribute a technology you've added locally, run:

```
stackwright registry share <slug>
```

This opens a GitHub issue with the YAML entry pre-filled. Accepted
entries become part of the synced registry available to everyone.

[registry]: https://github.com/joppe2001/stackwright-registry

## Configuration paths

stackwright respects XDG:

| Path | Purpose |
|------|---------|
| `$XDG_CONFIG_HOME/stackwright/registry.local.yaml` | User-added registry entries |
| `$XDG_CONFIG_HOME/stackwright/registry.cache.yaml` | Synced registry cache (24h TTL) |
| `$XDG_CONFIG_HOME/stackwright/logos/`               | Fetched tech logos (visual mode) |

Defaults to `~/.config/stackwright/` and `~/.cache/stackwright/` when
the XDG vars are unset.

## Local development

```
go build -o stackwright .      # build binary
go test ./...                  # run tests
./stackwright --detect         # verify terminal capabilities

# For npm wrapper development against a locally-built binary:
mkdir -p npm/bin/binary/$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/x86_64/amd64/')
cp stackwright npm/bin/binary/$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/x86_64/amd64/')/
STACKWRIGHT_SKIP_DOWNLOAD=1 (cd npm && npm link)
```

## License

MIT
