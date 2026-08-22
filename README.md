# starcat-weekly-api

<!-- starcat-promo:start -->
<div align="center">
<a href="https://starcat.ink"><img src="https://raw.githubusercontent.com/starcat-app/starcat-pro/main/banner.webp" width="100%" alt="Starcat" /></a>

<p><strong>Self-hostable support API for Starcat weekly project feeds and discovery pipeline.</strong></p>
<p>Starcat is a native macOS app that turns GitHub Stars into a searchable, organized and AI-assisted local knowledge base. Version 1.4.0 includes README rendering, knowledge-base RAG, GitHub notifications, My Projects, library and repository insights, macOS desktop widgets, tags and private notes, release tracking, repository health signals, AI summaries, semantic search, browser plugins, Alfred / uTools / Raycast search integrations, and self-hostable support APIs.</p>

<a href="https://github.com/starcat-app/homebrew-starcat"><img src="https://img.shields.io/badge/Install%20with-Homebrew-FBBF24?style=for-the-badge&logo=homebrew&logoColor=white" width="220" alt="Install with Homebrew"/></a>
<br/>
<sub><a href="./README-ZH.md">中文说明</a></sub>
</div>

<div align="center">
<a href="https://starcat.ink"><img src="https://img.shields.io/badge/website-starcat.ink-38BDF8?style=flat&color=blue" alt="website"/></a>
<a href="https://github.com/starcat-app/starcat-pro"><img src="https://img.shields.io/badge/support-starcat--pro-lightgrey.svg?style=flat&color=blue" alt="support"/></a>
<a href="https://github.com/starcat-app/homebrew-starcat"><img src="https://img.shields.io/badge/install-homebrew-lightgrey.svg?style=flat&color=blue" alt="homebrew"/></a>
<a href="https://github.com/starcat-app/starcat-localization"><img src="https://img.shields.io/badge/localization-open-lightgrey.svg?style=flat&color=blue" alt="localization"/></a>
</div>

<div align="center">
<img width="900" src="https://raw.githubusercontent.com/starcat-app/starcat-pro/main/main.webp" alt="Starcat main window"/>
</div>

**Preferred install method:**

```bash
brew tap starcat-app/starcat
brew trust starcat-app/starcat
brew install --cask starcat
```

**Useful links:**

- Home and downloads: https://starcat.ink
- Mac App Store: search for Starcat for GitHub
- Current Direct build: https://starcat.ink/downloads/Starcat-1.4.0-arm64.dmg
- Public support and release notes: https://github.com/starcat-app/starcat-pro
- Starcat App Homebrew tap: https://github.com/starcat-app/homebrew-starcat
- CLI / MCP: [starcat-cli](https://github.com/starcat-app/starcat-cli) / [Homebrew tap](https://github.com/starcat-app/homebrew-starcat-cli)
- AI Agent Skill: https://github.com/starcat-app/starcat-skill
- Browser plugins: [Chrome](https://github.com/starcat-app/starcat-chrome-plugin) / [Safari](https://github.com/starcat-app/starcat-safari-plugin)
- Launcher integrations: [Alfred](https://github.com/starcat-app/starcat-alfred-workflow) / [uTools](https://github.com/starcat-app/starcat-utools-plugin) / [Raycast](https://github.com/starcat-app/starcat-raycast-extension)
- Documentation: https://github.com/starcat-app/starcat-docs
- Website source: https://github.com/starcat-app/starcat-site
- Localization: https://github.com/starcat-app/starcat-localization

**Self-hostable support APIs:**

- [starcat-sharing-api](https://github.com/starcat-app/starcat-sharing-api)
- [starcat-trending-api](https://github.com/starcat-app/starcat-trending-api)
- [starcat-weekly-api](https://github.com/starcat-app/starcat-weekly-api)
- [starcat-wiki-api](https://github.com/starcat-app/starcat-wiki-api)
- [starcat-recommend-api](https://github.com/starcat-app/starcat-recommend-api)
- [starcat-discovery-api](https://github.com/starcat-app/starcat-discovery-api)

> Starcat provides hosted defaults for normal users. This API is open source so advanced users can inspect it, run it locally, or deploy their own instance.
<!-- starcat-promo:end -->

Starcat Weekly is a backend service that aggregates GitHub projects from
[Ruan Yifeng's Weekly](https://github.com/ruanyf/weekly), [zread.ai](https://zread.ai),
Show HN, HelloGitHub, and controlled manual intelligence sources, then serves them to
the [Starcat](https://starcat.ink) frontend through a unified REST API.

## R-01 Upgrade Notes

This project has completed the R-01 contract upgrade:
- **Expanded fields**: The `projects` table now includes 14+5 GitHub metadata fields.
- **API versioning**: All business endpoints have moved to `/api/v1/*`.
- **Standardized responses**: Every response uses the `{ "schema_version": <version>, "data": ... }` envelope; multi-source bulk responses use schema v2.
- **Authentication**: `Bearer Token` authentication is required through the `Authorization` header.
- **Token pool**: `GITHUB_TOKENS` supports rotation across multiple tokens.

## v0.5 R-02 (ZRead Source)

- **Unified list**: `GET /api/v1/repos?source=zread`; the separate public list endpoint has been removed
- **New table**: `zread_trending` (introduced in 0.5.0; decision ① keeps it separate from `projects`)
- **New cron job**: Fetches the public zread JSON endpoint every Monday at 06:00 UTC and writes the results to the database

## v0.6 AI Discovery (Show HN)

- Show HN is included as the fixed `discovery` source in `GET /api/v1/repos` and bulk responses; no separate public endpoint remains.
- `POST /internal/sync/discovery`: Allows administrators to trigger a sync manually using separate `ADMIN_API_KEYS`.
- The data source is the official Hacker News Firebase API. The collector does not parse HTML or depend on Algolia.
- The collector queues multiple Show HN submissions for the same repository as unified source events. A shared worker performs GitHub enrichment asynchronously.
- The current pipeline does not call an LLM. The legacy Discovery-specific tables remain only as migration rollback evidence and are no longer dual-written.

## Weekly Multi-Source Ingestion

- `weekly / zread / discovery / hellogithub / ai_intelligence` all write to the unified source event model. `GET /api/v1/repos/bulk` uses schema v2 to return the dynamic source catalog for sources with public repository events, unified source entries, and pin order.
- Collectors and manual imports write only batches and items within a SQLite transaction. After commit, they wake the background worker, which calls the GitHub API outside the transaction boundary.
- The worker scans once at startup, processes immediately when it receives an in-memory signal, and performs a fallback scan every 15 minutes. Transient failures are retried with 15- and 30-minute backoff intervals, then removed after three failed attempts.
- HelloGitHub supports incremental featured ingestion, monthly issue reconciliation, and resumable historical volume backfill. The backfill checkpoint is stored in the database.
- `POST /internal/imports` accepts every enabled source with `manual_import_enabled=true`. Administrators can create a manual source through `POST /internal/sources`; it is immediately available to subsequent imports.
- Manual events use `manual:<source_code>:<owner/repo>` as their stable identity. Reimporting the same repository in the same source updates the existing event even when the title, source URL, or batch idempotency key changes; different sources stay independent.
- Weekly supports multiple global pinned projects. The admin endpoint atomically replaces their order with the complete `gh_repo_ids` list.

### Safe Startup and Weekly Versions

- Only a brand-new empty database starts the initial `weekly / zread / discovery / hellogithub` collectors. A service restart, or a local instance opened from a production database backup, registers cron jobs but does not replay historical collectors. Scheduled jobs and explicit administrator sync endpoints remain available.
- Weekly synchronization uses the SHA-256 hash of each Markdown source file as its version and batch idempotency component. Git clone timestamps, volume restores, and file copies therefore cannot create a false full re-import.
- Existing databases receive the `content_hash` migration with an empty value. On the next successful weekly sync, each legacy issue is silently baselined to its current source content without parsing or GitHub enrichment; later content changes are queued normally. This intentionally favors preserving GitHub quota over replaying unknown pre-upgrade changes.

## Quick Start

### Local Development

```bash
# 1. Prepare the configuration file
cp .env.example .env
# Edit .env and set API_KEYS and GITHUB_TOKENS

# 2. Download dependencies
go mod download

# 3. Run
go run ./cmd/server/

# 4. Test the API (requires an API key)
API_KEY="your-key-from-env"
curl -H "Authorization: Bearer $API_KEY" http://localhost:5003/api/v1/ping
curl -H "Authorization: Bearer $API_KEY" http://localhost:5003/api/v1/repos?page=1\&page_size=5
```

### Docker

```bash
docker build -t starcat-weekly-api .
docker run -p 5003:5003 \
  --env-file .env \
  -v $(pwd)/data:/data \
  starcat-weekly-api
```

### Deploy to Fly.io

```bash
# Set production secrets
fly secrets set \
  API_KEYS="sk-starcat-prodKey1,..." \
  ADMIN_API_KEYS="sk-starcat-adminKey1,..." \
  GITHUB_TOKENS="ghp_token1,ghp_token2" \
  STORE_FILE="/data/weekly.db" \
  REPO_DIR="/data/weekly-repo"

fly deploy
```

## Configuration (.env)

| Variable | Description |
|------|------|
| `PORT` | Server port (default: 5003) |
| `STORE_FILE` | SQLite database path |
| `REPO_DIR` | Path for the Weekly git clone |
| `API_KEYS` | Comma-separated allowlist of API keys for Bearer authentication |
| `ADMIN_API_KEYS` | Dedicated admin keys for source sync, bulk imports, and pin management; must never be distributed with the client |
| `GITHUB_TOKENS` | Comma-separated pool of GitHub PATs |
| `DISCOVERY_CRON` | Discovery cron schedule; defaults to minute 17 of every hour |
| `HELLOGITHUB_CRON` | HelloGitHub incremental featured ingestion cron; defaults to 06:31 UTC daily |
| `HELLOGITHUB_RECONCILE_CRON` | HelloGitHub monthly issue reconciliation cron; defaults to 07:29 UTC on the 29th of each month |
| `HELLOGITHUB_FEATURED_MAX_PAGES` | Maximum pages for incremental featured ingestion; defaults to 3 |

## API (v1)

All business endpoints require the `Authorization: Bearer <API_KEY>` request header.

### Connectivity Check

```
GET /api/v1/ping
```

This endpoint requires Bearer authentication. A successful response contains `data.service = "weekly"`, the release-tag-derived `data.version`, and `data.ok = true`; Starcat uses it specifically for the "Test Connection" action in Settings.

### Aggregated Repository List

```
GET /api/v1/repos?page=1&page_size=20&lang=Go&source=hellogithub&sort=stars&order=desc
```

`source` is optional and accepts the fixed values `weekly`, `zread`, `discovery`, `hellogithub`, and `ai_intelligence`. If omitted, the endpoint returns all sources. `sort`, `order`, and `lang` are also optional.

### Full Bulk Snapshot

```
GET /api/v1/repos/bulk
```

This response uses schema v2. Its `data` object contains the dynamic `sources` catalog for sources with public repository events, aggregated `repos`, and `languages`. Starcat uses this snapshot to filter by source and language, sort, and paginate locally.

### Aggregated Repository Details

```
GET /api/v1/repos/{gh_repo_id}
```

The details include the repository's unified source entries. `gh_repo_id` is the numeric GitHub repository ID.

### Aggregated Language List

```
GET /api/v1/repos/languages
```

### Multi-Source Admin Endpoints

All endpoints use `Authorization: Bearer <ADMIN_API_KEY>`:

```text
GET  /internal/sources?manual_import=true
POST /internal/sources
POST /internal/sources/hellogithub/sync
GET  /internal/ingest-batches/{batch_id}
POST /internal/imports
GET  /internal/imports/{batch_id}
GET  /internal/repos/search?q=owner/repo&limit=20
GET  /internal/pins
POST /internal/pins
```

Create a manual source category (code must match `^[a-z][a-z0-9_]{1,31}$`):

```json
{
  "code": "developer_tools",
  "display_name_zh": "开发工具",
  "display_name_en": "Developer Tools"
}
```

Example bulk import. The endpoint persists the request before returning `202 Accepted`, and GitHub enrichment runs asynchronously:

```json
{
  "source_code": "ai_intelligence",
  "idempotency_key": "news-20260716-001",
  "repositories": [
    {
      "owner": "acme",
      "repo": "agent",
      "title": "AI Agent",
      "source_url": "https://example.com/news"
    }
  ]
}
```

HelloGitHub historical backfill request:

```json
{
  "mode": "backfill",
  "from_volume": 1,
  "to_volume": null,
  "idempotency_key": "hellogithub-history-v1"
}
```

The pin endpoint accepts the complete ordered list. An empty array clears all pins:

```json
{ "gh_repo_ids": [123, 456, 789] }
```

### ZRead and AI Discovery Sources

ZRead and Show HN AI Discovery are both part of the unified Weekly feed. They no longer have separate public list or detail endpoints:

```http
GET /api/v1/repos?source=zread&page=1&page_size=30
Authorization: Bearer <API_KEY>

GET /api/v1/repos?source=discovery&page=1&page_size=30
Authorization: Bearer <API_KEY>
```

Use the admin sync endpoints to refresh fixed collector sources immediately:

```http
POST /internal/sync/weekly
POST /internal/sync/zread
POST /internal/sync/discovery
Authorization: Bearer <ADMIN_API_KEY>
```

These endpoints consume GitHub quota, so they do not accept regular `API_KEYS`.

### Health Check (No Authentication)

```
GET /healthz
```

## Technology Stack

- **Go 1.23** — `net/http` standard library
- **goldmark** — Markdown AST parsing
- **modernc.org/sqlite** — Pure Go SQLite with no CGO, suitable for cross-compilation into a Docker scratch image
- **robfig/cron/v3** — Scheduled synchronization
- **Docker + Fly.io** — Multi-stage build with 256 MB of memory

## Testing

```bash
# Run all tests
go test ./...

# Run parser / spider unit tests only
go test ./internal/parser/ -v
go test ./internal/spider/ -v
```
