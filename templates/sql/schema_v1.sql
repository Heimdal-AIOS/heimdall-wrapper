PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
BEGIN;
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);
INSERT OR IGNORE INTO schema_migrations(version) VALUES (1);

CREATE TABLE IF NOT EXISTS projects (
  id INTEGER PRIMARY KEY,
  root TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);
INSERT OR IGNORE INTO projects(root, created_at) VALUES ('{{PROJECT_ROOT}}', datetime('now'));

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  project_id INTEGER NOT NULL,
  started_at TEXT NOT NULL,
  profile TEXT NOT NULL,
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

-- Files/Flatfiles schema (MVP)
CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,               -- 'dir'|'file'
  note TEXT,
  aicom TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(project_id, path),
  FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS file_tags (
  id INTEGER PRIMARY KEY,
  file_id INTEGER NOT NULL,
  tag TEXT NOT NULL,
  FOREIGN KEY(file_id) REFERENCES files(id)
);

CREATE TABLE IF NOT EXISTS file_lines (
  id INTEGER PRIMARY KEY,
  file_id INTEGER NOT NULL,
  lineno INTEGER NOT NULL,
  content TEXT NOT NULL,
  side TEXT,
  aicom TEXT,
  UNIQUE(file_id, lineno),
  FOREIGN KEY(file_id) REFERENCES files(id)
);
COMMIT;

