// Package github 薄包装 starcat-api-kit/github，保留 weekly 原有类型名与构造签名。
//
// 业务侧继续用 github.RepoResponse / NewClient(pool, limiter)；实现已收敛到 kit，
// 避免 weekly / trending / sharing / discovery 再复制一份 GetRepo。
package github

import (
	"context"
	"net/http"
	"time"

	kitgithub "github.com/starcat-app/starcat-api-kit/github"
	"github.com/starcat-app/starcat-weekly-api/internal/tokenpool"
)

// 与历史代码兼容的错误 / 类型别名。
var (
	ErrRepoNotFound = kitgithub.ErrRepoNotFound
	ErrRateLimited  = kitgithub.ErrRateLimited
)

// RepoResponse 是历史名称；底层为 kit 中立 Repo DTO。
type RepoResponse = kitgithub.Repo

// HTTPError 透传 kit 错误类型。
type HTTPError = kitgithub.HTTPError

// RateLimitHandler 透传 kit 限流器。
type RateLimitHandler = kitgithub.RateLimitHandler

// NewRateLimitHandler 创建限流器。
func NewRateLimitHandler(minInterval time.Duration) *RateLimitHandler {
	return kitgithub.NewRateLimitHandler(minInterval)
}

// Client 包装 kit Client。
type Client struct {
	inner *kitgithub.Client
}

// NewClient 创建 GitHub API 客户端（与历史签名一致）。
func NewClient(pool *tokenpool.Pool, limiter *RateLimitHandler) *Client {
	return &Client{inner: kitgithub.NewClient(kitgithub.Options{
		UserAgent: "starcat-weekly-api",
		Pool:      pool,
		Limiter:   limiter,
	})}
}

// SetBaseURL 覆盖 API 基础 URL（测试用）。
func (c *Client) SetBaseURL(url string) { c.inner.SetBaseURL(url) }

// SetHTTPClient 覆盖 HTTP 客户端（测试用）。
func (c *Client) SetHTTPClient(client *http.Client) { c.inner.SetHTTPClient(client) }

// GetRepo 调 GET /repos/{owner}/{repo}。
func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*RepoResponse, error) {
	return c.inner.GetRepo(ctx, owner, repo)
}

// GetReadme 调 README 接口。
func (c *Client) GetReadme(ctx context.Context, owner, repo string) (string, error) {
	return c.inner.GetReadme(ctx, owner, repo)
}
