# Pickup Notes — Heimdal AI:OS Wrapper

Date: [set by editor]

## What we did today
- Refactor: split `cmd/heimdal/main.go` into focused files:
  - `entry.go` (entry/dispatch), `shell_shim.go` (interactive shell),
    `run_cmd.go`, `app_cmd.go`, `config_cmd.go`, `log_cmd.go`, `helpers.go`.
- README: added Inside‑First section and `shell.json` example with `"inject_preprompt": true`.
- Virtual FS UX:
  - Added virtual CWD with `vcd` and made `cd` map to it in shell shims (zsh/bash).
  - `pwd`, `ls`, `cat`, `mkdir`, `newfile`, `append`, `mv`, `rm` now resolve paths relative to virtual CWD.
  - Implemented path normalization (`.`, `..`) for VFS commands.

## Open decisions / direction
- Storage model for file contents:
  - Move to single BLOB per file (`file_content.data`) to match OS semantics.
  - Keep `file_lines` as secondary index for annotate/side metadata.

## Next work items (pickup plan)
1) DB migration v2
   - Create `file_content(file_id INTEGER PRIMARY KEY, data BLOB NOT NULL DEFAULT '')`.
   - Backfill from existing `file_lines` by joining ordered lines with `\n`.
2) Update commands to BLOB path
   - `cat`: stream BLOB bytes.
   - `newfile`: create file and write initial BLOB.
   - `append`: read BLOB, append (with `\n` only when needed), write back atomically.
   - Maintain `file_lines` from BLOB on write (transaction) or lazy‑rebuild.
3) Add `write` command
   - Usage: `echo "text" | heimdal write <path>` (overwrite from stdin).
4) Docs & wiki
   - Document VFS, `vcd`/`cd`, and BLOB semantics in docs + wiki pages.
5) Tests (smoke)
   - VFS navigation (`vcd`, `pwd`, relative paths), `newfile`+`cat`, `append`, `write`, `mv`, `rm -r`.

## Quick verify commands
- Build: `make build`
- Open project shell: `./bin/heimdal project-open demo3`
- Try VFS:
  - `pwd` → `/`
  - `vcd project-folder` then `pwd` → `/project-folder`
  - `newfile notes/todo.txt --content "hello"` then `cat notes/todo.txt`
  - `append notes/todo.txt --text "more"` then `cat notes/todo.txt`

## Questions to confirm
- Size expectations for files; do we need chunking/compression now?
- Rebuild `file_lines` eagerly on every write vs. lazy on demand?
- Any reserved characters/encoding concerns for binary files?

