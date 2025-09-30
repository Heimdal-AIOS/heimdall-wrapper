# Heimdal — AI:OS Wrapper CLI

Heimdal is a developer‑first OS wrapper for AI coders. It runs on top of your host OS and gives any CLI (e.g., Claude, Gemini) a consistent “AI:OS” universe with passthrough to your normal environment.

## Features
- Wrapper shell with visible prompt prefix: `heimdal shell` keeps full OS behavior inside an AI:OS session.
- Shorthand run: `heimdal <app> [args...]` (alias for `run <app>`), works with any CLI in `PATH`.
- App manifests: declarative `apps/<name>.yaml` for `cmd`, `args`, `env` (and future policies).
- Universe sessions: per‑run/session context with `HEIMDAL_*` env vars and context files.
- Built‑in wiki (RAG manpages): `wiki.json` + `heimdal aioswiki search/show/init` (also available as `wiki`).

## Quick Start
- Requirements: Go 1.21+
- Build: `make build`
- Shell: `./bin/heimdal shell` (prefix `[hd]` by default)
- Run any app: `./bin/heimdal claude -- help`
- Manage apps:
  - Add: `./bin/heimdal app add claude --cmd claude`
  - List: `./bin/heimdal app ls`
  - Remove: `./bin/heimdal app rm claude`
- Wiki:
  - Init: `./bin/heimdal aioswiki init` (uses repo `wiki.json` if present, else `~/.heimdall/wiki.json`)
  - Search: `./bin/heimdal aioswiki search "ai-os"`
  - Show: `./bin/heimdal aioswiki show "Welcome to Heimdal AI:OS"`
 - Projects (outside Heimdal):
   - Init: `./bin/heimdal project-init demo`
   - Open: `./bin/heimdal project-open demo`
   - Pack: `./bin/heimdal project-pack demo -o demo.aiosproj.zip`
   - Unpack: `./bin/heimdal project-unpack demo.aiosproj.zip --dest ~/.heimdall/projects`

## Manifests (apps/<name>.yaml)
```yaml
name: gemini
cmd: gemini
args: ["--project", "demo"]
env:
  GEMINI_API_KEY: ${GEMINI_API_KEY}
```
If no manifest exists, `heimdal <app>` falls back to running `<app>` from `PATH` inside the universe.

## Universe Sessions
- Env: `HEIMDAL=1`, `HEIMDAL_UNIVERSE=1`, `HEIMDAL_SESSION`, `HEIMDAL_CONTEXT_DIR`, `HEIMDAL_WORKDIR`.
- Context files: `~/.heimdall/sessions/<id>/context/` (repo_files.txt, docs_files.txt, system.md).
- Prompt: customize with `--prompt-prefix="[heim] "`.

## Indefra (Inside‑First)
- Primær UX foregår inde i projektets Heimdal‑shell: `./bin/heimdal project-open <navn>` åbner et miljø med `[hd:<navn>]`‑prompt.
- Almindelige OS‑kommandoer mappes til DB‑understøttede varianter: `mkdir`, `newfile`, `ls`, `cat`, `mv`, `rm`, `pwd` (+ korte aliaser) for at arbejde mod projektets `project.sqlite`.
- Wiki kun via `aioswiki` (alias `wiki`); direkte adgang til `wiki.json` og Heimdal‑supportfiler bliver gardet i shell‑shims.
- Preprompt/instruktioner: `heimdal_instructions.txt` skrives til `$HEIMDAL_CONTEXT_DIR`. Hvis `inject_preprompt` er `true` i `shell.json`, injiceres filens indhold på stdin, når apps startes via `heimdal run` eller inde fra projektshellen.
- Hurtig demo inde i et projekt:
  - `./bin/heimdal project-open demo`
  - `mkdir src // første mappe`
  - `newfile src/main.py --content "print('hello')" @@demo`
  - `ls src` og `cat src/main.py`

## Profiles
- `--profile=permissive|restricted` flag exists. Current MVP is permissive; policy enforcement (network/FS) will arrive in later iterations.

## Project Structure
- `cmd/heimdal/` (CLI), `internal/` (config, manifest, universe, wiki), `apps/`, `docs/`, `Makefile`, `wiki.json`.
 - Project bundles: `~/.heimdall/projects/<name>.aiosproj/` containing `project.sqlite`, `rc/`, `meta.json`.

## Roadmap (high‑level)
- Session audit log + `log tail`.
- Adapters for popular AI CLIs (Claude, Gemini).
- Policy enforcement for `restricted` profile.
- Richer wiki/RAG and context providers.

## Shell Config
- Configure via `shell.json` in repo root or `~/.heimdall/shell.json`.
- Keys: `shell` (`zsh`|`bash`), `rc_mode` (`project-only`|`project-then-os`|`os-then-project`), `project_rc_dir`, `virtual_path`, `prompt_template` (token `__VPATH__`), `entry_echo`.
- Project rc files: `<project_rc_dir>/.zshrc` or `<project_rc_dir>/bashrc`.
- See wiki page: “Shell Configuration (AI:OS)” via `./bin/heimdal aioswiki show "Shell Configuration (AI:OS)"`.

### Eksempel: `shell.json` med preprompt
```json
{
  "shell": "zsh",
  "rc_mode": "project-then-os",
  "project_rc_dir": "rc",
  "virtual_path": true,
  "prompt_template": "__VPATH__ %(!.#.$)",
  "entry_echo": "Velkommen til Heimdal",
  "inject_preprompt": true
}
```

## CLI Help
- Outside Heimdal (no session): `./bin/heimdal --help` shows external commands (project-init/open/info, project-pack/unpack, aioswiki, app, run).
- Inside Heimdal ([hd] prompt): `heimdal --help` shows in-session commands (shell shorthands, aioswiki, app, run) and notes for external actions.
