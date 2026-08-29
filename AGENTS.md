# AGENTS.md — starcat-weekly-api

> **唯一协作规范源**：本仓库根目录 `AGENTS.md` 是本项目协作规范的唯一正文维护源。
> 开工前还必须阅读并遵守上级 [`../AGENTS.md`](../AGENTS.md) 的跨仓规则。

## 项目概述

Starcat Weekly 后端：聚合阮一峰周刊（ruanyf/weekly）、zread、Show HN（discovery 来源）、HelloGitHub 与受控人工情报（`ai_intelligence`）中的 GitHub 项目，经统一 REST API 提供给 Starcat 前端。多来源写入通用来源事件，GitHub enrich 由后台 Worker 异步完成。生产经 `starcat-api` 聚合部署。

## 技术栈

- Go 1.25.0 · `net/http`（纯标准库 HTTP）
- `github.com/yuin/goldmark`（Markdown AST 解析）
- `modernc.org/sqlite`（纯 Go，无 CGO）
- `github.com/robfig/cron/v3`（定时同步）
- `github.com/joho/godotenv`
- `github.com/starcat-app/starcat-api-kit` v0.3.0
- Docker + Fly.io（历史独立 App `starcat-weekly-api`）

## 关键目录

```
cmd/server/             # 入口，装配中间件与 TokenPool
server/                 # 可导出装配（聚合网关引用）
internal/
  model/                # project、repo_card、envelope、各来源 model
  middleware/auth.go    # Bearer 鉴权
  tokenpool/            # GitHub PAT 池
  store/sqlite.go       # createSchema 建表
  store/migrations.go   # runMigrations / schemaMigration 追加 migration
  parser/               # 阮一峰周刊 Markdown 解析（weekly 采集）
  source/               # 来源目录定义（catalog）与 HelloGitHub collector/backfill
  spider/               # zread 抓取
  scheduler/            # weekly / zread / discovery / hellogithub cron 调度
  ingest/               # 通用 ingest Worker
  discovery/            # Show HN collector
  enricher/             # GitHub 14+5 字段补全 + ratelimit
  handler/              # weekly、repos、bulk、pins、imports 等
REPO_DIR/               # 默认 ./.weekly-repo，ruanyf/weekly git clone
scripts/deploy.sh
Makefile
```

## 开发与测试命令

```bash
cp .env.example .env          # API_KEYS、ADMIN_API_KEYS、GITHUB_TOKENS 必填
make deps                     # 或 go mod download
make run                      # go run ./cmd/server，PORT=5003
make build                    # bin/server
make check                    # fmt-check + vet + test（PR 前）
make docker-build
```

CI（`.github/workflows/go.yml`）：gofmt · vet · docker build · test -race · build。

README 补充验证：
```bash
curl -H "Authorization: Bearer $API_KEY" http://localhost:5003/api/v1/ping
curl -H "Authorization: Bearer $API_KEY" "http://localhost:5003/api/v1/repos?page=1&page_size=5"
```

环境变量见 `.env.example`：`STORE_FILE`、`METRICS_STORE_FILE`、`REPO_DIR`、`ADMIN_API_KEYS`、各来源 cron（`ZREAD_TRENDING_CRON`、`DISCOVERY_CRON`、`HELLOGITHUB_*`）等。

## R-01 架构约束（保留）

- **统一契约**：响应必须包入 envelope（`internal/model/envelope.go`：`schema_version` + `data`）；bulk 多来源为 schema v2。
- **鉴权**：所有 `/api/v1/*` 必须 `Authorization: Bearer <key>`；`/healthz` 不鉴权。
- **Token 池**：调用 GitHub API 必须经由 `internal/tokenpool/tokenpool.go`（或 api-kit 等价实现）；环境变量为 `GITHUB_TOKENS`（非旧单值 `GITHUB_TOKEN`）。
- **硬边界**：扩充字段须区分核心 `StarcatRepoCardDTO` 与 weekly 扩展段；勿把来源专属字段污染核心 DTO。
- **Admin Key**：`ADMIN_API_KEYS` 管 `/internal/sync/*`、`/internal/imports`、置顶等；**不得与 `API_KEYS` 相同**（客户端会分发 API_KEYS）。

## 多来源与迁移约束（保留）

- 固定来源：`weekly` / `zread` / `discovery` / `hellogithub` / `ai_intelligence`；统一 `GET /api/v1/repos?source=...`，无独立公开列表端点。各来源写入 `repo_source_events` 统一来源事件模型。
- Collector 只在 SQLite transaction 内写 batch/items；commit 后唤醒 Worker，Worker 在事务外调 GitHub API。
- Worker：启动扫描 + 内存信号 + 15 分钟兜底；失败 15/30 分钟退避，最多 3 次。
- HelloGitHub：featured 增量、月刊对账、可恢复历史 volume 回填；checkpoint 存数据库。
- **人工来源**：内建 `ai_intelligence` 默认 `manual_import_enabled=true` 且对公开 feed 开放；管理员还可经 `POST /internal/sources` 创建动态 manual source，随后 `POST /internal/imports` 导入。
- **Schema 迁移**：`internal/store/migrations.go` 的 `runMigrations()` 按 `schemaMigration` 版本号只追加执行未落库 migration（写入 `schema_migrations`）；禁止回写已执行 migration、禁止要求用户删库重建。
- **zread**：原始抓取写入 `zread_events` 表；对外与 bulk 统一经 `repo_source_events`（`source_code=zread`），不再使用旧 `zread_trending` 表模型。

## 代码与架构约束

- 共享 auth/envelope 优先 api-kit；业务专属 ingest/source 逻辑留本仓。
- 可选 `WIKI_API_URL` + `WIKI_API_KEY` 预热 wiki 探测缓存；未配置则静默跳过。
- 改 parser / ingest 后跑 `make check`；`internal/parser/testdata/` 为单测夹具。

## 安全与数据边界

- 禁止入库：`.env`、`weekly.db`、`weekly-metrics.db`、`.weekly-repo/`、`bin/`、`logs/`。
- `ADMIN_API_KEYS` 不得随客户端分发或写入公开文档。

## 部署与发布禁令

未经 dong4j 明确授权，禁止：`make release`、`scripts/deploy.sh`、`fly deploy`、`git push`/`git tag`。生产 Fly 仅经 `starcat-api`。

## Commit 规范

- `feat:` / `fix:` / `test:` / `chore:` 等 Conventional Commits 前缀
- 每个 commit 末尾 `Closes HOM-176`
