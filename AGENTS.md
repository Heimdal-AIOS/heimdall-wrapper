# Repository Guidelines

## Project Structure & Module Organization
- `docs/` — User and design documentation. Keep diagrams and ADRs here.
- `src/` — Application code (create if missing). Use one top-level package or module (e.g., `heimdall_wrapper`).
- `tests/` — Automated tests mirroring `src/` structure.
- `scripts/` — Small, reproducible CLI helpers for local tasks.
- `examples/` — Minimal runnable samples that demonstrate common flows.

## Build, Test, and Development Commands
- Prefer `scripts/` or `Makefile` targets to hide tool specifics.
- Examples (adapt to toolchain in use):
  - Build: `make build` or `scripts/build` — compiles/bundles artifacts.
  - Test: `make test` or `scripts/test` — runs the full test suite with coverage.
  - Lint/Format: `make lint && make fmt` or `scripts/lint`/`scripts/fmt`.
  - Run locally: `make run` or `scripts/dev`.

## Coding Style & Naming Conventions
- Keep modules small and cohesive; one responsibility per file.
- Filenames and directories: use `snake_case`; exported types/entities: `PascalCase`; variables/functions: `lower_snake_case`.
- Document public APIs with concise docstrings/comments.
- Run formatters and linters before pushing; fix all errors and high‑severity warnings.

## Testing Guidelines
- Place tests under `tests/` mirroring `src/` paths.
- Naming: `test_<unit>.py` or `<unit>.spec.<ext>` depending on language.
- Aim for meaningful unit tests and pragmatic integration tests; add regression tests for every bug fix.
- Include coverage when available; gate critical paths with tests.

## Commit & Pull Request Guidelines
- Use Conventional Commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`.
- Keep commits focused and atomic; include rationale in the body when non‑obvious.
- PRs must include: clear summary, linked issues, test evidence (output or coverage), and any screenshots for UX changes.
- Draft PRs are welcome early; convert to ready when passing CI and reviewed.

## Security & Configuration
- Do not commit secrets. Use `.env` locally and provide a scrubbed `.env.example`.
- Keep configuration in code or environment variables; document required keys in `docs/`.

## Agent‑Specific Notes
- Follow this file and any nested `AGENTS.md` files; deeper files take precedence.
- Keep changes minimal, scoped, and consistent with existing patterns. Prefer small PRs.
