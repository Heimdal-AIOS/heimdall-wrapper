# OS-WrapperHeimdall — Vision og DevOps Plan

## Formål
- Bygge en Go-baseret CLI, der opfører sig som en "OS-wrapper": brugeren starter `heimdall`, som præsenterer en shells/CLI-oplevelse, hvor alt virker som normalt OS – men kører inde i wrapperen.
- Målet er at kunne "wrappe" andre CLI-apps (fx Claude-Code, Gemini-Cli) og udvide med nye kapabiliteter uden at forstyrre værts-OS'et.

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

## CLI-design (forslag)
- `heimdall shell` — starter wrapper-shell.
- `heimdall app add <navn> --cmd "gemini" --args "--project X"` — registrér app.
- `heimdall app ls|rm <navn>` — list/slet registrerede apps.
- `heimdall run <navn> [args...]` — kør wrapped app med politikker/ miljø.
- `heimdall log tail` — stream logs fra nuværende session.
 - `heimdal <app> [args...]` — shorthand alias til `heimdall run <app>`.
 - Globale flag: `--profile=permissive|restricted` (default: permissive).

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

## Kørselsmodel: alias og profiler
- Alias: `heimdal <app> [args...]` kører som `heimdall run <app>`.
- Permissiv profil (default): injicerer miljø og logger, men blokerer ikke OS-adgang.
- Restricted profil: håndhæver policies (netværk/FS) i det omfang OS'et understøtter det.
- App-opslag: find manifest `apps/<app>.yaml`; hvis mangler, kør `<app>` direkte fra PATH i permissiv mode med logging.
- Fejl: hvis binær ikke findes, returnér klar fejl med forslag til installation/sti.

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

## Sikkerhed & Isolation
- Ingen hemmeligheder i repo; brug `.env` og `.env.example`.
- Politikker pr. app (netværk/FS) håndhæves bedst via OS-mekanismer: namespaces/seccomp (Linux), sandbox-profil (macOS), Job Objects (Windows). MVP kan starte med "policy-as-documentation" og løbende hårdhærdning.

## Roadmap
1) MVP shell + app-manifest + logging.
2) Politikmotor (tilladelser), cache og profil-styring.
3) Interaktiv TUI (valgfrit), forbedret DX, telemetry opt-in.
4) Sandboxing pr. OS, robust plugin-API, katalog over færdige wrappers.

## Åbne spørgsmål
- Krav til cross-platform paritet? Hvilke OS prioriteres først?
- Hvilken grad af isolation kræves før GA?
- Skal wrappers kunne orkestrere flere CLI'er i samme pipeline-session?
