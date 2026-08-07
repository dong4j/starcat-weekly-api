# Changelog

本项目的所有重要变更都会记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [2.0.0] - 2026-08-07

### Changed
- 导出可装配 `server` 包；依赖 `starcat-api-kit`。
- `internal/github` 改为 kit 薄包装；`/api/v1/ping` 改用 kit httputil。
- `server.FromEnv` 改用 kit `env`（含 `LookupCSV` / `Int`）。

## [Unreleased]

### Added
- **AI Discovery v0.6 (2026-06-11)**：完成 Show HN 后端 API 与定时流水线。
  - 使用 Hacker News 官方 `/v0/showstories` + `/v0/item/{id}` API，支持从 `url` 和自发布 `text` 提取 GitHub 仓库。
  - 新增 `discovery_repos` / `discovery_submissions` 双表，保留同 repo 多次投稿历史。
  - 新增 GitHub metadata + README excerpt enrich，并复用现有 Token Pool / RateLimitHandler。
  - 新增 OpenAI-compatible 单标签分类器，带 prompt injection 防护、confidence 阈值和可真正解冻的 7 天冷却状态机。
  - 新增 `GET /api/v1/discovery`、`GET /api/v1/discovery/{owner}/{repo}`、`POST /internal/sync/discovery`。
  - Admin sync 使用独立 `ADMIN_API_KEYS`，不接受随客户端分发的普通 API Key。
  - API 使用 endpoint 专用 `{repo, discovery}` DTO，不修改跨服务共享 `StarcatRepoCardDTO` / `Meta` 契约。
- **R-03 (2026-06-11)**：新增 `GET /api/v1/ping` 端点，专给 Starcat 客户端「测试连接」按钮用。
  - 走 BearerAuth 中间件，鉴权通过返回 200 + envelope `{data: {service: "weekly", ok: true}}`；
    无效 / 缺失 Key → 401；服务故障 → 5xx。
  - 实现：`internal/handler/ping.go` + `internal/handler/ping_test.go`（7 case）。
  - 设计意图：让客户端用一个语义明确的端点同时验「服务可达 + Key 正确」，
    不再借用业务 endpoint 的 GET 来当 auth probe。
  - 跨项目约定：本 `ping.go` 与 trending / sharing / wiki 三个项目「除 import path 外 byte-level 一致」。

## [0.5.2] - 2026-06-10

### Changed
- **路径重命名（dong4j 拍板）**：把阮一峰周刊端点从「通用 projects 命名」改为「weekly 命名空间」，与 zread 周 trending 端的命名风格对齐：
  - `GET /api/v1/projects` → `GET /api/v1/weekly`
  - `GET /api/v1/projects/{owner}/{repo}` → `GET /api/v1/weekly/{owner}/{repo}`
  - `POST /internal/sync` → `POST /internal/sync/weekly`
  - `GET /api/v1/issues` / `GET /api/v1/issues/{number}` / `GET /api/v1/zread` / `POST /internal/sync/zread` 维持不动
- **客户端同步**：Starcat 主项目 `AppEndpoints.swift` 同步更新 3 处 string literal（`projects` / `projectsByOwnerRepo`）+ 4 个源文件注释同步。**breaking change**（旧端点返 404，客户端必须同步升级）
- **Rationale**：① weekly 是周更内容，命名空间与「每周精选」语义对齐 ② 与 `/api/v1/zread` / `/internal/sync/zread` 形成清晰的「weekly/zread 双数据源」结构 ③ 内部符号名（`HandleProjectsV1` / `WeeklyHandler` / `Projects` 模型）**不重命名**（避免与 6+ 历史章节冲突，留待后续清理）

## [0.5.1] - 2026-06-10

### Changed
- **全新服务语态（dong4j 拍板）**：删除 `migrateV1` / `migrateV2` / `migrateV3` / `PRAGMA user_version` 机制。`store/sqlite.go` 改为单文件 `createSchema(s)` 函数，三张表（`weekly_issues` / `projects` / `zread_trending`）一次性建好。任何现存 `weekly.db` 直接 `rm` 即可，不做 destructive migration。`projects` 表原本 V2 加的 14 个 GitHub 字段直接在 createSchema 里建好（无 ALTER 路径）。

## [0.5.0] - 2026-06-10 (R-02 zread 接入 weekly-api)

### Added
- **新端点** `GET /api/v1/zread?week=this|last|YYYY-MM-DD&limit=20`：
  返回 zread 公开周 trending 列表（无鉴权公开 JSON 端点，决策 ② 独立端点不动现有）
- **新表** `zread_trending`（原 V3 migration,0.5.1 改用 createSchema 一次性建好）：11 zread 字段 + 14 enricher 字段 + 3 v0.4.1
  跨年回溯字段 + 4 索引（决策 ① 独立建表不合并 projects）
- **新 store 方法** `UpsertZreadTrending` / `QueryZreadTrending` / `LookupZreadWikiID`
- **新 cron 任务** 周一 06:00 UTC 拉 zread 周 trending，写库后调 enricher.EnrichAll 补 14 字段
- **新 spider** `internal/spider/zread.go` + `zread_types.go` + `zread_year_infer.go`
  （从 starcat-trending-api 迁入，import 路径改造）
- **年份推断异常告警** `InferYear()` 阈值 `>1`（v0.4.1 dong4j 反馈），
  `log.Printf` 告警含原始 MM/DD，2027 年元旦前后可观察
- **funcName 锁** 防止并发跑同一 cron 任务
- **响应 envelope** `data` 字段新增 `ZreadTrendingEnvelope { week_label, week_start, week_end,
  fetched_at, items }`；envelope `Meta` 通过 data 内嵌的 `week_label` 区分 zread 数据源
  （**不污染** envelope.go 4 份共享件）

### Changed
- **scheduler.go** 新增 `runZreadFetch()` + `SyncZread()` 导出方法
- **.env.example** 新增 `ZREAD_TRENDING_CRON`（注释保留，默认值在 cron.go 硬编码）

### Notes
- **不影响** 现有 `/api/v1/projects` / `/api/v1/issues` / `/api/v1/issues/{number}` 端点
  （决策 ② 严格遵守 — 行为 100% 兼容）
- 客户端 WeeklyView source picker 与 merged 视图将在**独立 PR** 处理

## [1.0.0] - 2026-06-08 (R-01 v1.2 完成)

### Changed (R-01 Transformation)
- **Breaking Change**: 移除旧版 `/api/weekly/*` 接口，全量迁移至 `/api/v1/*`。
- **Breaking Change**: 从 `GITHUB_TOKEN` 迁移至 `GITHUB_TOKENS` (Token Pool)。
- **API 升级**: 所有业务响应现在包入统一的 `{ "schema_version": 1, "data": ... }` envelope。
- **鉴权增强**: 引入 `Bearer Token` 鉴权，需在请求头中携带 `Authorization: Bearer <key>`。
- **字段补齐**: `projects` 表新增 14 个 GitHub 元数据字段（`gh_repo_id`, `forks`, `watchers`, `subscribers`, `owner_avatar`, `homepage`, `license_spdx`, `is_archived`, `is_fork`, `is_private`, `default_branch`, `open_issues`, `pushed_at`, `updated_at`, `created_at`）。
- **配置管理**: 引入 `joho/godotenv` 支持从 `.env` 加载环境变量。
- **存储演进**: 引入 SQLite `migrateV2` (基于 `PRAGMA user_version`) 自动处理表结构升级（0.5.1 拆除,改为 createSchema 一次性建好）。
- **限流优化**: 接入 Quota-aware Token Pool 和 `RateLimitHandler` 主动退避。

### Added
- 新增单项目聚合接口 `GET /api/v1/projects/{owner}/{repo}`。
- 新增 `StarcatRepoCardDTO` 统一 DTO 结构。

## [1.0.0] - 2026-06-08 (阮一峰周刊首发)

### Added
- 阮一峰周刊（ruanyf/weekly）每小时自动同步（git clone/pull + goldmark 解析）
- 项目列表 API（`GET /api/weekly/projects`，支持分页 / 期号 / 语言 / 排序筛选）
- 期号列表 API（`GET /api/weekly/issues`）
- 期号详情 API（`GET /api/weekly/issues/{id}`）
- 手动同步接口（`POST /internal/sync`）
- 健康检查接口（`GET /healthz`）
- 基于 modernc.org/sqlite 的本地存储（无 CGO，可交叉编译进 scratch 镜像）
- GitHub API 元数据补全（stars / language / description，异步批量）
- robfig/cron/v3 定时调度
- Docker + Fly.io 多阶段部署（256MB 内存）
- GitHub Actions CI 工作流（`go vet` / `gofmt` / 编译 / 单测）
- Dependabot 自动依赖更新
- Issue / PR 模板
- 贡献指南和变更日志
- 内部版本号包 (`internal/version`, 暴露 `version.Version` 常量)

[Unreleased]: https://github.com/starcat-app/starcat-weekly-api/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/starcat-app/starcat-weekly-api/compare/v1.0.0...v0.5.0
[1.0.0]: https://github.com/starcat-app/starcat-weekly-api/releases/tag/v1.0.0
