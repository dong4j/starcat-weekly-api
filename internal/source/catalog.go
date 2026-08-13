// Package source 定义 Weekly 内建采集来源及能力边界。
//
// Crawler 来源仍必须随代码发布；管理员可通过受保护接口追加 manual 来源，
// 但不能创建采集器或修改内建来源的能力位。
package source

import "github.com/starcat-app/starcat-weekly-api/internal/model"

const (
	IngestModeCrawler = "crawler"
	IngestModeManual  = "manual"
)

// Definition 是代码内内建来源的完整声明，也是 migration 初始化 source_catalog
// 的单一来源。Enabled 表示公开 feed 可见，ManualImportEnabled 表示允许管理批量录入。
type Definition struct {
	Code                string
	DisplayNameZH       string
	DisplayNameEN       string
	IconKey             string
	IngestMode          string
	SortOrder           int
	Enabled             bool
	ManualImportEnabled bool
}

// Definitions 按客户端展示顺序声明首期来源。
//
// HelloGitHub 与 AI 情报在本次功能中已经具备对应客户端实现，因此 migration
// 直接启用；只有 AI 情报开放 manual import，防止 Skill 绕过 crawler 写入其他来源。
var Definitions = []Definition{
	{Code: model.SourceWeekly, DisplayNameZH: "阮一峰周刊", DisplayNameEN: "Weekly", IconKey: "ruanyf", IngestMode: IngestModeCrawler, SortOrder: 10, Enabled: true},
	{Code: model.SourceZread, DisplayNameZH: "ZRead", DisplayNameEN: "ZRead", IconKey: "zread", IngestMode: IngestModeCrawler, SortOrder: 20, Enabled: true},
	{Code: model.SourceDiscovery, DisplayNameZH: "Hacker News", DisplayNameEN: "Hacker News", IconKey: "hackernews", IngestMode: IngestModeCrawler, SortOrder: 30, Enabled: true},
	{Code: model.SourceHelloGitHub, DisplayNameZH: "HelloGitHub", DisplayNameEN: "HelloGitHub", IconKey: "hellogithub", IngestMode: IngestModeCrawler, SortOrder: 40, Enabled: true},
	{Code: model.SourceAIIntelligence, DisplayNameZH: "AI 情报", DisplayNameEN: "AI Intelligence", IconKey: "ai-intelligence", IngestMode: IngestModeManual, SortOrder: 50, Enabled: true, ManualImportEnabled: true},
}

// Find 返回固定来源定义，不识别的来源必须由调用方拒绝。
func Find(code string) (Definition, bool) {
	for _, definition := range Definitions {
		if definition.Code == code {
			return definition, true
		}
	}
	return Definition{}, false
}
