package model

import "time"

// SourceDescriptor 是 bulk v2 返回给客户端的动态来源目录项。
type SourceDescriptor struct {
	Code          string `json:"code"`
	DisplayNameZH string `json:"display_name_zh"`
	DisplayNameEN string `json:"display_name_en"`
	IconKey       string `json:"icon_key"`
	SortOrder     int    `json:"sort_order"`
	Count         int    `json:"count"`
}

// SourceStatus 是管理端来源目录及队列运行状态。
type SourceStatus struct {
	SourceDescriptor
	IngestMode          string       `json:"ingest_mode"`
	Enabled             bool         `json:"enabled"`
	ManualImportEnabled bool         `json:"manual_import_enabled"`
	Pending             int          `json:"pending"`
	Processing          int          `json:"processing"`
	Retrying            int          `json:"retrying"`
	Discarded           int          `json:"discarded"`
	LastSuccessAt       string       `json:"last_success_at,omitempty"`
	LastFailureAt       string       `json:"last_failure_at,omitempty"`
	LatestBatch         *IngestBatch `json:"latest_batch,omitempty"`
	ActiveBackfill      *IngestBatch `json:"active_backfill,omitempty"`
}

// ManualSourceInput 是管理员在发布台创建人工来源时允许写入的最小字段集合。
// 服务端固定其 ingest mode、图标和权限位，避免客户端伪造 crawler 来源。
type ManualSourceInput struct {
	Code          string
	DisplayNameZH string
	DisplayNameEN string
}

// SourceEntry 是仓库在某个来源中的最新代表事件。
// Payload 只承载来源专属补充字段，通用筛选不得依赖其中内容。
type SourceEntry struct {
	SourceCode string         `json:"source_code"`
	OccurredAt string         `json:"occurred_at"`
	SourceURL  string         `json:"source_url,omitempty"`
	Title      string         `json:"title,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	Rank       *int           `json:"rank,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// SourceEventInput 是 Collector 与 Worker 写入来源事实的内部模型。
// ExternalKey 必须在来源内部稳定，确保重试和服务重启不会产生重复事件。
type SourceEventInput struct {
	SourceCode  string
	ExternalKey string
	OccurredAt  time.Time
	SourceURL   string
	Title       string
	Summary     string
	Rank        *int
	Payload     map[string]any
}
