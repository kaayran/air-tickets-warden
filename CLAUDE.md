# Project instructions for Claude

## Dev database access

- For ad-hoc inspection of the dev database, use the read-only role only:
  `docker compose exec postgres psql -U warden_ro -d warden` (or
  `psql "$WARDEN_RO_DATABASE_URL"` if psql is installed on the host).
  Never run ad-hoc SQL as the `warden` app user; schema changes go through
  goose migrations under review.

## Git commits

- **Do NOT add a `Co-Authored-By: Claude ...` trailer to commit messages** in this repository, and do not add any other AI/agent co-author or "Generated with Claude Code" line. Write the commit message with the project's own attribution only.
