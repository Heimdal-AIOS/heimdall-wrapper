# Heimdall SQLite Project Feature — Designnotat (aligneret med lars.md)

## Formål
- AI:OS projekter er “datadrevne”: en `.sqlite` pr. projekt er sandheden; mappestruktur i Heimdal er en visning/illusion oven på relationer.
- Gem sessions, kørselshistorik, “flatfiles” (linjer), foldere (relationer), tags og annoteringer i DB.

## 1) Projektlivscyklus
- Opret: `heimdal --project "NAME"` eller inde i shell: `heimdal project-init NAME`.
- Resultat: udenfor skabes `NAME.sqlite`; inde i Heimdal skabes mappen `/NAME/` som AI:OS‑roden for projektet.
- Start projekt: `heimdal NAME <app>` åbner universet med `/NAME` som rod og med DB bundet til sessionen.

## 2) Datamodel: flatfiles, foldere og annotationer
- Flatfile = en “fil” hvor hver linje er et felt; folder = relation (samling af flatfiles).
- Annotationer på linjeniveau:
  - `// note` flytter teksten til sideløbende felt (`side`/note).
  - `@@tag1,tag2` lægger tags på linjen.
  - `::...::` markerer AICOM (AI‑DSL) feltet for kommunikation/styring.
- Kommandoer (eksempler):
  - `heimdal newfile path/name.type` → opretter flatfile + første linje.
  - `heimdal mkdir path/folder` → opretter relation/folder.
  - `heimdal annotate path/name.type --line N "// comment @@tag ::aicom::"` → opdaterer felter for linje N.

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

## CLI‑design (forslag)
- Projekt: `heimdal project-init NAME`, `heimdal project-open NAME`, `heimdal project-info`.
- Filer: `heimdal newfile path/name.type`, `heimdal mkdir path/folder`.
- Annotationer: `heimdal annotate path/name.type --line N "// .. @@tag ::dsl::"`.
- Historik: `heimdal run <app> [args...] --record`, `heimdal log tail --from-db`.

## 3) RAG
- RAG på tværs af felter og typer: søg i `file_lines.content/side/aicom` og `line_tags`.
- Start simpelt med FTS5 + tag‑filter; senere embeddings/semantic search.
- Mål: sprog‑neutral, DRY‑fremmende, intuitiv navigation efter funktion, klasse, tags.

## 4) Eksport
- Én samlet fil pr. type eller separat filstruktur.
- Håndtér afhængigheder via linter/rewriter, så dubletter undgås.

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
