# NEXT_SESSION.md — Handoff for Session 002

## Where we left off

Session 001 completed Phases Zero through Four (foundation).
Session 002 should continue with Milestone 0.2: storage integration tests and discovery plugin migration.

## Immediate next actions

### 1. Add PGBackend integration tests (testcontainers-go)

```bash
cd knowledge
go get github.com/testcontainers/testcontainers-go@latest
```

Create `knowledge/internal/storage/postgres_test.go`:
- Spin up a postgres container
- Run `001_initial_schema.up.sql`
- Run all Backend interface methods against it
- Verify RLS policies work correctly

### 2. Add refresh token endpoint

The `refresh_tokens` table exists in the schema (Session 001). Wire up:

1. Store JTI in `refresh_tokens` during `IssueTokens`
2. Add `POST /api/v1/refresh` handler in `api/handler.go`
3. The handler: validate refresh token → check JTI not revoked → revoke old JTI → issue new pair

### 3. Migrate first discovery plugin

Source: `~/Desktop/GIT/knowledge-tree-git/plugins/aws/`
Target: `~/Desktop/GIT/zerelio/knowledge/plugins/aws/`

Steps:
- Read the source plugin carefully (read-only source)
- Create new plugin module under `knowledge/plugins/aws/`
- Update import paths: `kt/plugins/aws` → `github.com/zerelio/zerelio/plugins/aws`
- Implement the `discovery.Plugin` interface
- Add plugin to `go.work`

### 4. Fix DiscoveryScope type/schema mismatch

`CURRENT_STATE.md` notes that `DiscoveryScope` Go type uses `LastRun *time.Time` and `Status string`, but the schema migration has `created_at`/`updated_at` columns. The PGBackend's `CreateScope` / `ListScopes` need alignment — either add fields to the Go type or drop them from the schema.

## Context to reload

- Read `AGENTS.md` for conventions and constraints
- Read `CURRENT_STATE.md` for exact implementation status
- Read `docs/architecture/self-review.md` for outstanding issues
- The source repo is at `~/Desktop/GIT/knowledge-tree-git` — read-only reference

## Compile check

```bash
cd /Users/simonbalazs/Desktop/GIT/zerelio/knowledge
GOWORK=off go test -buildvcs=false -race -count=1 ./...
```

Expected: 16 tests, all PASS, zero races.
