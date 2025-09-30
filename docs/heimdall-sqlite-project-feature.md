# Heimdall SQLite Project Feature — Designnotat

## Formål
- Et letvægts projekt‑lager i SQLite, som Heimdall/heimdal bruger til at gemme sessions, kørselshistorik, wiki‑referencer, artefakter og projektmetadata.
- Giver AI:OS et konsistent “hukommelseslag” pr. repo/projekt — uden ekstern DB.

## Anvendelser
- Spor `run`/`shell` sessions og kommandoer pr. projekt.
- Knyt wiki‑sider, noter og artefakter (logs, output‑filer) til en session/run.
- Gør det muligt at søge i historik og kontekst på tværs af tiden.

## Arkitektur & Placering
- Filsti: `./.heimdall/project.db` (repo‑lokalt). Fallback: `~/.heimdall/projects/<repo-hash>.db`.
- SQLite med WAL + `PRAGMA foreign_keys=ON`.
- Skriver atomisk; ingen netværkskrav; egnet til lokal udvikling og CI.

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
- `heimdal project init` — opretter `./.heimdall/project.db` og `schema_migrations`.
- `heimdal project info` — viser DB‑sti og statistik (sessions, runs).
- `heimdal project query '<SQL>'` — læsende forespørgsler (SELECT ...).
- `heimdal run <app> [args...] --record` — persister `runs` og knyt til aktiv `session`.
- `heimdal log tail --from-db` — stream seneste run fra DB (fallback til fil når offline).

## Integration i AI:OS
- Universe‑sessions (HEIMDAL_*) oprettes som `sessions` og genbruges for efterfølgende `run`.
- Wiki‑brug (aioswiki) kan logge `wiki_refs` (titel + snippet) for tracebarhed.
- Artefakter: gem CLI‑output/logs pr. run, evt. med max‑størrelse og oprydning.

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

