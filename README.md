# Heimdal — AI:OS Wrapper CLI

Heimdal is a developer‑first OS wrapper for AI coders. It runs on top of your host OS and gives any CLI (e.g., Claude, Gemini) a consistent “AI:OS” universe with passthrough to your normal environment.

## Features
- Wrapper shell with visible prompt prefix: `heimdal shell` keeps full OS behavior inside an AI:OS session.
- Shorthand run: `heimdal <app> [args...]` (alias for `run <app>`), works with any CLI in `PATH`.
- App manifests: declarative `apps/<name>.yaml` for `cmd`, `args`, `env` (and future policies).
- Universe sessions: per‑run/session context with `HEIMDAL_*` env vars and context files.
- Built‑in wiki (RAG manpages): `wiki.json` + `heimdal wiki search/show/init`.

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
  - Init: `./bin/heimdal wiki init` (uses repo `wiki.json` if present, else `~/.heimdall/wiki.json`)
  - Search: `./bin/heimdal wiki search "ai-os"`
  - Show: `./bin/heimdal wiki show "Welcome to Heimdal AI:OS"`

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

## Profiles
- `--profile=permissive|restricted` flag exists. Current MVP is permissive; policy enforcement (network/FS) will arrive in later iterations.

## Project Structure
- `cmd/heimdal/` (CLI), `internal/` (config, manifest, universe, wiki), `apps/`, `docs/`, `Makefile`, `wiki.json`.

## Roadmap (high‑level)
- Session audit log + `log tail`.
- Adapters for popular AI CLIs (Claude, Gemini).
- Policy enforcement for `restricted` profile.
- Richer wiki/RAG and context providers.
