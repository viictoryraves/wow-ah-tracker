# Project rules

Go daemon (`backend/`) + React app (`frontend/`) tracking WoW Forever auction prices. See `README.md` and `docs/`.

- Tests: `cd backend && go test -race ./...` · `cd frontend && npm test`. Both must pass before every commit.
- **Test-first (TDD) and KISS.** Write the failing test, then the simplest code. Never change a test just to make it pass.
- **After each feature, commit** in small logical commits (`feat(scope): subject`). Never mention Claude, AI or a
  co-author in a commit. Do not push unless asked.
- Keep `docs/functional-specification.md` and `docs/design-specification.md` in step with behaviour, in the same commit.
- Never commit secrets (`backend/.env`) or data (`backend/data/`). Blizzard terms: data kept at most 30 days, no ads.
- Redeploy only with `backend/scripts/install-daemon.sh`, and not around :22 past the hour (Blizzard's publication):
  a restart then loses that hour's sales comparison.
