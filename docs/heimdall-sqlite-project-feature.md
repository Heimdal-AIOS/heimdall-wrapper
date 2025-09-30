# Heimdall SQLite Project Feature — Designnotat (Indefra‑paradigme)

## Formål
- AI:OS projekter er “datadrevne”: en `.sqlite` pr. projekt er sandheden; mappestruktur i Heimdal er en visning/illusion oven på relationer.
- Gem sessions, kørselshistorik, “flatfiles” (linjer), foldere (relationer), tags og annoteringer i DB.

## 1) Projektlivscyklus
- Opret (ude): `heimdal project-init NAME` (opretter `NAME.aiosproj/` bundle med `project.sqlite`, `rc/`, `meta.json`).
- Åbn (ude): `heimdal project-open NAME` → inde i [hd:NAME] shell er rod “/” (virtuelt), og OS‑kommandoer er mappet til DB‑lag.
- Start AI‑coder (ude): `heimdal NAME <aicoder> [args…]` (preprompt kan injiceres; se nedenfor).

### Oprettelse (trin‑for‑trin)
1) Initier projektet
   - Kommandolinje: `heimdal project-init demo`
   - Forventet output: opretter filen `demo.sqlite` (udenfor Heimdal‑FS) og registrerer projektet.

2) Åbn AI:OS med projektrod
   - `heimdal demo shell` eller `heimdal project-open demo`
   - Inde i [hd]‑shell ses en logisk mappestruktur med `/demo/` som rod for AI:OS.

3) Kør apps i projektkontekst
   - `heimdal demo claude -- help` (shorthand) eller `heimdal project-open demo && heimdal claude -- help`

Eksempel (forventet struktur/artefakter)
```
host-filer:
  ./demo.sqlite                 # projektets sandhed (DB)

heimdal (inde i [hd]-sessionen):
  /demo/                        # AI:OS-rod for projektet
    files/                      # visning af flatfiles (illusorisk)
    cache/
    context/                    # session/context-filer
```

## 2) Indefra: Flatfiles, foldere og annotationer
- Flatfile = en “fil” hvor hver linje er et felt; folder = relation (samling af flatfiles).
- Annotationer på linjeniveau:
  - `// note` flytter teksten til sideløbende felt (`side`/note).
  - `@@tag1,tag2` lægger tags på linjen.
  - `::...::` markerer AICOM (AI‑DSL) feltet for kommunikation/styring.
- Inde i [hd]-shell mappes OS‑kommandoer til DB‑lag: `mkdir`, `ls`, `cat`, `mv`, `rm`, `pwd`, `newfile` (samt aliaser `mk`, `nf`, `ll`, `ct`, `ap`, `an`).
- Eksempler:
  - `mkdir path/folder @@docs ::Design:: //top`
  - `newfile path/name.type --content "Hello"`
  - `annotate path/name.type --line 1 "// note @@tag ::aicom::"`

## Skema (forslag)
```sql
-- versionsstyring
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);

CREATE TABLE IF NOT EXISTS projects (
  id INTEGER PRIMARY KEY,
  root TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,              -- HEIMDAL_SESSION
  project_id INTEGER NOT NULL,
  started_at TEXT NOT NULL,
  profile TEXT NOT NULL,            -- permissive|restricted
  context_dir TEXT NOT NULL,
  FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY,
  session_id TEXT NOT NULL,
  app TEXT NOT NULL,
  cmdline TEXT NOT NULL,
  started_at TEXT NOT NULL,
  exit_code INTEGER,
  FOREIGN KEY(session_id) REFERENCES sessions(id)
);

-- Flatfiles og foldere
CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  type TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(project_id, path),
  FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS file_lines (
  id INTEGER PRIMARY KEY,
  file_id INTEGER NOT NULL,
  lineno INTEGER NOT NULL,
  content TEXT NOT NULL,
  side TEXT,            -- fra // note
  aicom TEXT,           -- fra ::...::
  FOREIGN KEY(file_id) REFERENCES files(id),
  UNIQUE(file_id, lineno)
);

CREATE TABLE IF NOT EXISTS line_tags (
  id INTEGER PRIMARY KEY,
  line_id INTEGER NOT NULL,
  tag TEXT NOT NULL,
  FOREIGN KEY(line_id) REFERENCES file_lines(id)
);

CREATE TABLE IF NOT EXISTS wiki_refs (
  id INTEGER PRIMARY KEY,
  session_id TEXT NOT NULL,
  title TEXT NOT NULL,
  selection TEXT,
  FOREIGN KEY(session_id) REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS artifacts (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL,
  kind TEXT NOT NULL,               -- log|file|note
  path TEXT,
  content BLOB,
  FOREIGN KEY(run_id) REFERENCES runs(id)
);
```

## CLI‑design (indefra/ude)
- Ude (lifecycle): `heimdal project-init|open|pack|unpack|migrate`.
- Inde (arbejde): brug mappede kommandoer `mkdir/newfile/ls/cat/mv/rm/pwd` (+ aliaser) — de går via DB.
- Annotationer: `annotate path --line N "// note @@tag ::dsl::"`.

## 3) RAG & Wiki
- RAG på tværs af felter og typer: søg i `file_lines.content/side/aicom` og `line_tags`.
- Start simpelt med FTS5 + tag‑filter; senere embeddings/semantic search.
- Mål: sprog‑neutral, DRY‑fremmende, intuitiv navigation efter funktion, klasse, tags.
- Wiki kun via `aioswiki` (global `~/.heimdall/wiki.json`); direkte adgang til wiki.json/supportfiler er blokeret i shell.

## 4) Eksport
- Én samlet fil pr. type eller separat filstruktur.
- Håndtér afhængigheder via linter/rewriter, så dubletter undgås.

## 5) Preprompt/Instructions
- Heimdal genererer `$HEIMDAL_CONTEXT_DIR/heimdal_instructions.txt` når et projekt åbnes/køres.
- Sæt `"inject_preprompt": true` i `shell.json` (repo eller `~/.heimdall/shell.json`) for automatisk at injicere instruktionen på stdin til AI‑coders (ved `heimdal NAME <aicoder> …`).

## Sikkerhed & Privatliv
- Gem aldrig hemmeligheder; masker kendte nøgler (API_KEY, TOKEN, SECRET).
- Opt‑in for telemetri; alt er lokalt som standard.

## Migration & DevOps
- `schema_migrations` styrer versioner; `heimdal project migrate` anvender næste DDL.
- Backup/eksport: `sqlite3 ./.heimdall/project.db ".backup project.db.bak"`.

## Roadmap
- FTS5‑indeks for `runs.cmdline`/`artifacts.content` (hurtig søgning).
- Embeddings/semantic search (lokal) med offline indeks.
- Eksport til JSON/CSV for deling og rapportering.
