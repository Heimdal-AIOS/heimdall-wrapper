# OS-WrapperHeimdall — Vision og DevOps Plan

## Formål (indefra-paradigme)
- `heimdal` er et AI:OS‑lag, hvor hovedværdien ligger inde i projekt‑shellen. Udefra bruges kun til livscyklus (init/open/pack/unpack) og opstart af AI‑coders.
- Inde i [hd]-shell føles det som et OS: velkendte kommandoer mappes til DB‑lag og arbejder mod projektets sandhed.

## MVP-mål (fase 1)
- Interaktiv shell (`heimdall shell`) med TTY/PTY passthrough, så almindelige kommandoer køres på værts-OS.
- Kommando-proxy og audit-log: alle kommandoer kan logges/filtreres.
- Simpel app-wrapping: registrér eksterne CLI'er med alias og miljø (manifest).
- Konfiguration i `$HOME/.heimdall/config.yaml` + pr.-projekt `.heimdall.yaml`.

## Arkitektur (høj-niveau)
- Core (Go): proces-supervision, PTY, miljøhåndtering, logging, plugin-loader.
- Adapters: wrappers for specifikke CLI-apps (Claude-Code, Gemini-Cli).
- Config: Viper-baseret (YAML), isolerede profiler (fx `default`, `dev`, `prod`).
- Udvidelser: dynamiske "apps" defineret via manifest eller kompilerede plugins.

## CLI‑design
- Udefra (lifecycle):
  - `heimdal project-init <navn>`, `heimdal project-open <navn>`, `heimdal project-pack|unpack`, `heimdal project-migrate`.
  - Start AI‑coder med projekt: `heimdal <navn> <aicoder> [args…]` (kan injicere preprompt/instructions).
- Indefra (arbejde):
  - Mappede kommandoer: `mkdir`, `newfile`, `ls`, `cat`, `mv`, `rm`, `pwd` (+ aliaser `mk`, `nf`, `ll`, `ct`, `ap`, `an`).
  - Wiki kun via `aioswiki` (`search|show|init`).

## Manifest-eksempel (apps/<navn>.yaml)
```yaml
name: gemini
cmd: gemini
args: ["--project", "demo"]
env:
  GEMINI_API_KEY: ${GEMINI_API_KEY}
policies:
  network: allow
  filesystem:
    read: ["./", "$HOME/.config/gemini"]
    write: ["./.heimdall-cache"]
```

### Manifest-eksempel (apps/claude.yaml)
```yaml
name: claude
cmd: claude
args: []
env: {}
policies:
  network: allow
```

## Kørselsmodel: indefra og preprompt
- Indefra: brug mappede kommandoer til alt projektarbejde; OS‑kommandoer rammer DB‑laget.
- Udefra: brug kun lifecycle + opstart af AI‑coder. Sæt `"inject_preprompt": true` i `shell.json` for automatisk at injicere `$HEIMDAL_CONTEXT_DIR/heimdal_instructions.txt` til AI‑coders via stdin ved start.
- App‑opslag: bruger `apps/<navn>.yaml` hvis tilgængelig; ellers fallback til PATH.

## Wrapping af CLI-apps
- Claude-Code: `heimdall app add claude --cmd "claude"`; kør med `heimdal claude -- help`.
- Gemini-Cli: `heimdall app add gemini --cmd "gemini"`; kør med `heimdal gemini prompt.md`.
- Skift profil: `heimdal --profile=restricted gemini prompt.md`.
- Wrapperen injicerer miljø, standard-stier, logging og (i restricted) politikker pr. app.

## DevOps & Repo-struktur (forslag)
- `cmd/heimdall/` (main), `internal/` (core), `pkg/` (SDK), `apps/` (manifester), `scripts/` (build/test), `docs/`.
- Build: `go build ./cmd/heimdall` — statisk binær hvor muligt.
- Test: `go test ./...` — inkluder PTY/integrationstests bag build-tags.
- Lint/format: `golangci-lint run` og `gofmt -s -w .`.
- Release: GitHub Actions (build matrix), version via tags, udgiv binære artefakter.

## Sikkerhed & Wiki
- Ingen hemmeligheder i repo; brug `.env` og `.env.example`.
- Wiki-sti eksponeres ikke; direkte adgang til wiki.json og Heimdal supportfiler blokeres i shell. Al adgang går via `aioswiki`.

## Roadmap
1) MVP shell + app-manifest + logging.
2) Politikmotor (tilladelser), cache og profil-styring.
3) Interaktiv TUI (valgfrit), forbedret DX, telemetry opt-in.
4) Sandboxing pr. OS, robust plugin-API, katalog over færdige wrappers.

## Åbne spørgsmål
- Krav til cross-platform paritet? Hvilke OS prioriteres først?
- Hvilken grad af isolation kræves før GA?
- Skal wrappers kunne orkestrere flere CLI'er i samme pipeline-session?
