// Package server 导出 weekly-api 的可装配 HTTP 服务。
//
// 单仓部署走 cmd/server；聚合部署（starcat-api）import 本包并挂到网关。
// 业务实现仍在 internal/，本包只负责 env 装配、路由注册、后台 Worker 与调度器生命周期。
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	kitenv "github.com/starcat-app/starcat-api-kit/env"
	kitmetrics "github.com/starcat-app/starcat-api-kit/metrics"
	"github.com/starcat-app/starcat-weekly-api/internal/discovery"
	"github.com/starcat-app/starcat-weekly-api/internal/github"
	"github.com/starcat-app/starcat-weekly-api/internal/handler"
	"github.com/starcat-app/starcat-weekly-api/internal/ingest"
	"github.com/starcat-app/starcat-weekly-api/internal/middleware"
	"github.com/starcat-app/starcat-weekly-api/internal/notifier"
	"github.com/starcat-app/starcat-weekly-api/internal/scheduler"
	weeklysource "github.com/starcat-app/starcat-weekly-api/internal/source"
	"github.com/starcat-app/starcat-weekly-api/internal/store"
	"github.com/starcat-app/starcat-weekly-api/internal/tokenpool"
	"github.com/starcat-app/starcat-weekly-api/internal/version"
)

const defaultPort = "5003"

// Options 控制 weekly 服务装配。聚合网关可显式传入，单仓部署通常用 FromEnv。
type Options struct {
	Port                   string
	DBPath                 string
	MetricsStoreFile       string
	RepoDir                string
	APIKeys                []string
	AdminAPIKeys           []string
	GithubTokens           []string
	DiscoveryHNLim         int
	HelloGitHubMaxPages    int
	DiscoveryCron          string
	HelloGitHubCron        string
	HelloGitHubReconcile   string
	ZreadTrendingCron      string
	SkipListenLogEndpoints bool
}

// Service 是已装配的 weekly HTTP 服务（含 ingest Worker、HelloGitHub backfill 与 cron 调度器）。
type Service struct {
	opts       Options
	handler    http.Handler
	store      store.Store
	scheduler  *scheduler.Scheduler
	stopWorker context.CancelFunc
	metrics    *kitmetrics.Collector
	closeOnce  sync.Once
}

// Name 返回聚合网关识别用的稳定服务名。
func Name() string { return "weekly" }

// DefaultPort 返回单仓默认监听端口。
func DefaultPort() string { return defaultPort }

// FromEnv 从环境变量装配服务（与历史 cmd/server 行为一致）。
// 配置缺失时返回 error，由调用方决定如何记录/退出（不在此 log.Fatal）。
func FromEnv() (*Service, error) {
	apiKeys, err := kitenv.RequiredCSV("API_KEYS")
	if err != nil {
		return nil, fmt.Errorf("API_KEYS env is required (comma-separated list of valid API keys)")
	}

	adminKeys := kitenv.LookupCSV("ADMIN_API_KEYS")
	if len(adminKeys) == 0 {
		log.Println("[auth] ADMIN_API_KEYS not configured; source sync, imports, batch status, and pins are disabled")
	}

	githubTokens, err := githubTokensFromEnv()
	if err != nil {
		return nil, err
	}

	opt := Options{
		Port:                 kitenv.OrDefault("PORT", defaultPort),
		DBPath:               kitenv.OrDefault("STORE_FILE", "weekly.db"),
		MetricsStoreFile:     kitenv.OrDefault("METRICS_STORE_FILE", "weekly-metrics.db"),
		RepoDir:              kitenv.OrDefault("REPO_DIR", ".weekly-repo"),
		APIKeys:              apiKeys,
		AdminAPIKeys:         adminKeys,
		GithubTokens:         githubTokens,
		DiscoveryHNLim:       kitenv.Int("DISCOVERY_HN_LIMIT", 30),
		HelloGitHubMaxPages:  kitenv.Int("HELLOGITHUB_FEATURED_MAX_PAGES", 3),
		DiscoveryCron:        kitenv.OrDefault("DISCOVERY_CRON", "17 * * * *"),
		HelloGitHubCron:      kitenv.OrDefault("HELLOGITHUB_CRON", "31 6 * * *"),
		HelloGitHubReconcile: kitenv.OrDefault("HELLOGITHUB_RECONCILE_CRON", "29 7 29 * *"),
		ZreadTrendingCron:    kitenv.OrDefault("ZREAD_TRENDING_CRON", "0 6 * * *"),
	}
	return New(opt)
}

// New 按 Options 装配服务。
func New(opt Options) (*Service, error) {
	if strings.TrimSpace(opt.Port) == "" {
		opt.Port = defaultPort
	}
	if strings.TrimSpace(opt.MetricsStoreFile) == "" {
		opt.MetricsStoreFile = ":memory:"
	}
	if len(opt.APIKeys) == 0 {
		return nil, fmt.Errorf("APIKeys is required")
	}
	if len(opt.GithubTokens) == 0 {
		return nil, fmt.Errorf("GithubTokens is required (GITHUB_TOKENS or GITHUB_TOKEN)")
	}

	// 注意：NewBearerAuth 内部自动打 [auth] N keys loaded 启动日志（含日志脱敏），无需重复打印。
	authMW := middleware.NewBearerAuth(opt.APIKeys)
	adminAuthMW := middleware.NewBearerAuth(opt.AdminAPIKeys)

	// 注意：tokenpool.New 内部自动打 [token-pool] loaded N tokens 启动日志，无需重复打印。
	pool := tokenpool.New(opt.GithubTokens)

	s, err := store.NewSQLiteStore(opt.DBPath)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	metricsStore, err := kitmetrics.OpenSQLite(opt.MetricsStoreFile)
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("initialize metrics SQLite: %w", err)
	}
	metricsCollector, err := kitmetrics.NewCollector(kitmetrics.Config{Service: Name(), Store: metricsStore})
	if err != nil {
		_ = metricsStore.Close()
		_ = s.Close()
		return nil, fmt.Errorf("initialize metrics collector: %w", err)
	}
	metricsHandler := kitmetrics.NewHandler(Name(), metricsCollector.Store())

	ghClient := github.NewClient(pool, github.NewRateLimitHandler(720*time.Millisecond)) // 5000/h ≈ 720ms
	hnClient := discovery.NewHNClient(nil)
	wikiNotifier := notifier.NewWikiNotifier()

	bulkCache := handler.NewBulkCache()
	wakeSignal := ingest.NewWakeSignal()
	ingestService := ingest.NewService(s, wakeSignal)
	ingestWorker := ingest.NewWorker(s, ghClient, wakeSignal, bulkCache)
	discoveryCollector := discovery.NewCollector(hnClient, ingestService, opt.DiscoveryHNLim)
	helloGitHubCollector := weeklysource.NewHelloGitHubCollector(
		weeklysource.NewHelloGitHubClient(nil), ingestService, opt.HelloGitHubMaxPages)
	helloGitHubBackfill := weeklysource.NewHelloGitHubBackfillManager(
		s, weeklysource.NewHelloGitHubClient(nil), ingestService)

	workerContext, stopWorker := context.WithCancel(context.Background())
	go ingestWorker.Run(workerContext)
	go helloGitHubBackfill.Run(workerContext)

	sch := scheduler.New(s, ingestService, wikiNotifier, opt.RepoDir, discoveryCollector, helloGitHubCollector,
		opt.DiscoveryCron, opt.HelloGitHubCron, opt.HelloGitHubReconcile, opt.ZreadTrendingCron)

	wh := handler.NewWeeklyHandler(s, sch.Sync, sch.SyncZread)
	rh := handler.NewReposHandlerWithBulkCache(s, bulkCache)
	dh := handler.NewDiscoveryHandler(s, sch.SyncDiscovery)
	hgh := handler.NewHelloGitHubHandler(sch.SyncHelloGitHub, sch.ReconcileHelloGitHub, helloGitHubBackfill)
	ih := handler.NewImportsHandler(ingestService, s)
	ph := handler.NewPinsHandler(s, bulkCache)

	// Register routes (Go 1.22+ style)
	// 注意：authMW.Wrap 接受 http.Handler。把 method value (func(w,r)) 显式包装为
	// http.HandlerFunc 让它满足 http.Handler 接口（Go 不支持隐式转换）。
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", wh.Healthz) // Health check (unauthenticated)

	mux.Handle("GET /api/v1/ping", authMW.Wrap(handler.HandlePingV1(version.Service, version.Version)))

	mux.Handle("GET /api/v1/repos", authMW.Wrap(http.HandlerFunc(rh.HandleListV1)))
	mux.Handle("GET /api/v1/repos/bulk", authMW.Wrap(handler.HandleBulkV1(s, bulkCache)))
	mux.Handle("GET /api/v1/repos/languages", authMW.Wrap(http.HandlerFunc(rh.HandleLanguagesV1)))
	mux.Handle("GET /api/v1/repos/{gh_repo_id}", authMW.Wrap(http.HandlerFunc(rh.HandleDetailV1)))
	mux.Handle("GET /internal/stats", authMW.Wrap(handler.HandleOperationalStats(s)))
	mux.Handle("GET /internal/metrics/summary", authMW.Wrap(http.HandlerFunc(metricsHandler.HandleSummary)))
	mux.Handle("GET /internal/metrics/timeseries", authMW.Wrap(http.HandlerFunc(metricsHandler.HandleTimeseries)))
	mux.Handle("GET /internal/metrics/routes", authMW.Wrap(http.HandlerFunc(metricsHandler.HandleRoutes)))
	mux.Handle("GET /internal/metrics/status-codes", authMW.Wrap(http.HandlerFunc(metricsHandler.HandleStatusCodes)))

	mux.Handle("POST /internal/sync/weekly", adminAuthMW.Wrap(http.HandlerFunc(wh.HandleAdminSync)))
	mux.Handle("POST /internal/sync/zread", adminAuthMW.Wrap(http.HandlerFunc(wh.HandleZreadSync)))
	mux.Handle("POST /internal/sync/discovery", adminAuthMW.Wrap(http.HandlerFunc(dh.HandleAdminSync)))
	mux.Handle("POST /internal/sources/{source_code}/sync", adminAuthMW.Wrap(http.HandlerFunc(hgh.HandleSourceSync)))
	mux.Handle("POST /internal/rebuild-aggregates", adminAuthMW.Wrap(http.HandlerFunc(rh.HandleRebuildAggregates)))
	mux.Handle("GET /internal/sources", adminAuthMW.Wrap(http.HandlerFunc(ih.HandleSources)))
	mux.Handle("POST /internal/sources", adminAuthMW.Wrap(http.HandlerFunc(ih.HandleCreateSource)))
	mux.Handle("POST /internal/imports", adminAuthMW.Wrap(http.HandlerFunc(ih.HandleCreate)))
	mux.Handle("GET /internal/imports/{batch_id}", adminAuthMW.Wrap(http.HandlerFunc(ih.HandleBatch)))
	mux.Handle("GET /internal/ingest-batches/{batch_id}", adminAuthMW.Wrap(http.HandlerFunc(ih.HandleBatch)))
	mux.Handle("GET /internal/ingest-batches", adminAuthMW.Wrap(handler.HandleRecentIngestBatches(s)))
	mux.Handle("GET /internal/repos/search", adminAuthMW.Wrap(http.HandlerFunc(ph.HandleSearch)))
	mux.Handle("GET /internal/pins", adminAuthMW.Wrap(http.HandlerFunc(ph.HandleList)))
	mux.Handle("POST /internal/pins", adminAuthMW.Wrap(http.HandlerFunc(ph.HandleReplace)))

	go sch.Start()

	if !opt.SkipListenLogEndpoints {
		log.Printf("starcat-weekly-api %s endpoints ready", version.Version)
		log.Printf("V1 Endpoints (authenticated):")
		log.Printf("  GET  /api/v1/ping               - Connectivity probe for Starcat client")
		log.Printf("  GET  /api/v1/repos              - List aggregated repos (paginated)")
		log.Printf("  GET  /api/v1/repos/bulk         - One-shot full payload (repos + languages, gzip + ETag 304)")
		log.Printf("  GET  /api/v1/repos/{id}         - Get aggregated repo detail")
		log.Printf("  GET  /api/v1/repos/languages    - List aggregated languages")
		log.Printf("  POST /internal/sync/weekly      - Trigger manual sync (阮一峰周刊)")
		log.Printf("  POST /internal/sync/zread       - Trigger manual sync (zread 周 trending)")
		log.Printf("  POST /internal/sync/discovery   - Trigger manual sync (ADMIN_API_KEYS)")
		log.Printf("  POST /internal/sources/{code}/sync - Trigger HelloGitHub incremental/backfill sync")
		log.Printf("  POST /internal/rebuild-aggregates - Recompute source aggregates")
		log.Printf("  GET  /internal/sources           - List fixed sources and ingest status")
		log.Printf("  POST /internal/sources           - Create a manual import source")
		log.Printf("  POST /internal/imports           - Enqueue a manual repository batch")
		log.Printf("  GET  /internal/imports/{id}      - Inspect ingest batch status")
		log.Printf("  GET  /internal/repos/search      - Search repos for pinning")
		log.Printf("  GET/POST /internal/pins          - Read or replace ordered Weekly pins")
	}

	return &Service{
		opts:       opt,
		handler:    metricsCollector.Wrap(middleware.CORS(mux)),
		store:      s,
		scheduler:  sch,
		stopWorker: stopWorker,
		metrics:    metricsCollector,
	}, nil
}

// Handler 返回已包 CORS 的根 handler，可供聚合网关挂载。
func (svc *Service) Handler() http.Handler { return svc.handler }

// Addr 返回建议监听地址（":port"）。
func (svc *Service) Addr() string { return ":" + svc.opts.Port }

// Close 停止 ingest Worker、HelloGitHub backfill 与 cron 调度器，并关闭 SQLite。
// 与 cmd/server 收到 SIGINT/SIGTERM 时的清理顺序一致；可安全重复调用。
func (svc *Service) Close() error {
	var closeErr error
	svc.closeOnce.Do(func() {
		if svc.stopWorker != nil {
			svc.stopWorker()
		}
		if svc.scheduler != nil {
			svc.scheduler.Stop()
		}
		if svc.metrics != nil {
			closeErr = svc.metrics.Close()
		}
		if svc.store != nil {
			if err := svc.store.Close(); closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

func githubTokensFromEnv() ([]string, error) {
	if tokens := kitenv.LookupCSV("GITHUB_TOKENS"); len(tokens) > 0 {
		return tokens, nil
	}
	if old := kitenv.OrDefault("GITHUB_TOKEN", ""); old != "" {
		log.Println("[token-pool] migrating legacy GITHUB_TOKEN to GITHUB_TOKENS (single token)")
		return []string{old}, nil
	}
	return nil, fmt.Errorf("GITHUB_TOKENS or GITHUB_TOKEN env required (at least 1 GitHub PAT)")
}
