// Package memory 是「写入」这一层的业务规则。
//
// 为什么单独一层（而不是塞进 store）：
//   - 去重的返回语义（重复 = 返回已有 id，不是错误）
//   - embedding 失败要不要回滚（不回滚：记忆先落地，向量可后补）
//   - TTL、默认值、错误分类
//
// 这些是产品规则，换 Postgres 时不该变；S12 的 MCP 工具只调这一层。
package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/embed"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/pkg/types"
)

// DefaultEmbedTimeout 是写入侧 embedding 的超时。
//
// 比检索侧（3s）宽松：写入可以等，检索不能卡。
const DefaultEmbedTimeout = 30 * time.Second

// DefaultWarmupTimeout 是启动 warmup 的超时。
//
// 冷加载 qwen3-embedding:0.6b 实测约 20s；给到 90s 是为了慢机器/首次下载后
// 仍能完成预热，而不是让后台 goroutine 提前放弃。
const DefaultWarmupTimeout = 90 * time.Second

// Input 是一次写入的入参。
type Input struct {
	TenantID string
	Scope    string
	Content  string
	Kind     types.Kind
	Source   string
	Metadata map[string]any
	// TTL > 0 时设置过期时间；0 表示永不过期。
	TTL time.Duration
}

// Result 是写入结果。
type Result struct {
	ID string
	// Duplicate 表示同租户同 scope 下已有相同内容，本次没有写入新记录。
	Duplicate bool
	// Embedded 表示向量已成功写入。
	Embedded bool
	// EmbeddingFailed 表示记忆已保存，但向量化失败（可稍后 Reembed 补）。
	EmbeddingFailed bool
	// EmbeddingErr 是失败原因的文本（只用于日志/排查，不做判断依据）。
	EmbeddingErr string
}

// Service 实现写入规则。
type Service struct {
	store         store.Store
	embedder      embed.Embedder
	embedTimeout  time.Duration
	queryTimeout  time.Duration
	warmupTimeout time.Duration
	now           func() time.Time
}

// Option 配置 Service。
type Option func(*Service)

// WithEmbedTimeout 设置写入侧 embedding 超时。
func WithEmbedTimeout(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.embedTimeout = d
		}
	}
}

// WithQueryTimeout 设置检索侧 embedding 超时（默认 3s）。
func WithQueryTimeout(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.queryTimeout = d
		}
	}
}

// WithWarmupTimeout 设置启动 warmup 的超时（默认 90s）。
func WithWarmupTimeout(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.warmupTimeout = d
		}
	}
}

// WithClock 注入时钟（测试用）。
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// New 构建 Service。embedder 可以为 nil，表示当前不做向量化（仅关键词检索可用）。
func New(st store.Store, e embed.Embedder, opts ...Option) *Service {
	s := &Service{
		store:         st,
		embedder:      e,
		embedTimeout:  DefaultEmbedTimeout,
		queryTimeout:  DefaultQueryTimeout,
		warmupTimeout: DefaultWarmupTimeout,
		now:           time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Warmup 预加载 embedding 模型，不写入任何记忆。
//
// 为什么需要：qwen3-embedding:0.6b 冷加载约 20s，而检索侧只有 3s 预算。
// 服务启动后在后台调用一次，让「空闲后第一次查询」不再被冷加载打成 degraded。
// 失败不致命（Ollama 可能还没起来），检索仍会按 D-015 降级到关键词。
func (s *Service) Warmup(ctx context.Context) error {
	if s.embedder == nil {
		return nil
	}
	wctx, cancel := context.WithTimeout(ctx, s.warmupTimeout)
	defer cancel()

	_, err := embed.EmbedQuery(wctx, s.embedder, "memalyze warmup")
	return err
}

// Add 写入一条记忆。
//
// 语义：
//   - 内容为空 → ErrInvalid
//   - 已存在相同内容 → 返回已有 ID 且 Duplicate=true（不是错误）
//   - embedding 失败 → 记忆保留，EmbeddingFailed=true，**不返回错误**
//   - 其余存储错误 → 原样返回
func (s *Service) Add(ctx context.Context, in Input) (Result, error) {
	if s.store == nil {
		return Result{}, fmt.Errorf("%w: nil store", store.ErrInvalid)
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return Result{}, fmt.Errorf("%w: empty content", store.ErrInvalid)
	}

	m := &types.Memory{
		TenantID: in.TenantID,
		Scope:    in.Scope,
		Content:  content,
		Kind:     in.Kind,
		Source:   in.Source,
		Metadata: in.Metadata,
	}
	if in.TTL > 0 {
		exp := s.now().Add(in.TTL)
		m.ExpiresAt = &exp
	}

	id, err := s.store.Add(ctx, m)
	res := Result{ID: id}
	switch {
	case errors.Is(err, store.ErrDuplicate):
		res.Duplicate = true
		return res, nil // 已有同内容记忆：不重复向量化
	case err != nil:
		return Result{}, err
	}

	s.embedInto(ctx, id, content, &res)
	return res, nil
}

// Delete 软删除一条记忆。
//
// 语义：
//   - 不存在（或不属于该租户）→ store.ErrNotFound，不伪装成功
//   - 已删除 → 幂等成功（便于客户端重试）
//   - 空 tenant / id → ErrInvalid
func (s *Service) Delete(ctx context.Context, tenantID, memoryID string) error {
	if s.store == nil {
		return fmt.Errorf("%w: nil store", store.ErrInvalid)
	}
	tenantID = strings.TrimSpace(tenantID)
	memoryID = strings.TrimSpace(memoryID)
	if tenantID == "" {
		return fmt.Errorf("%w: tenant_id required", store.ErrInvalid)
	}
	if memoryID == "" {
		return fmt.Errorf("%w: memory id required", store.ErrInvalid)
	}
	return s.store.SoftDelete(ctx, tenantID, memoryID)
}

// Reembed 重新计算某条记忆的向量（换模型、或上次 embedding 失败时补算）。
//
// 与 Add 不同：这是显式操作，失败会返回错误（调用方需要知道没成功）。
func (s *Service) Reembed(ctx context.Context, tenantID, memoryID string) (Result, error) {
	if s.store == nil {
		return Result{}, fmt.Errorf("%w: nil store", store.ErrInvalid)
	}
	if s.embedder == nil {
		return Result{}, fmt.Errorf("%w: no embedder configured", store.ErrInvalid)
	}
	m, err := s.store.Get(ctx, tenantID, memoryID)
	if err != nil {
		return Result{}, err
	}
	if m.Status == types.StatusDeleted {
		return Result{}, fmt.Errorf("%w: memory %s is deleted", store.ErrInvalid, memoryID)
	}

	res := Result{ID: memoryID}
	s.embedInto(ctx, memoryID, m.Content, &res)
	if res.EmbeddingFailed {
		return res, fmt.Errorf("memory: reembed %s: %s", memoryID, res.EmbeddingErr)
	}
	return res, nil
}

// embedInto 尽力向量化并落库；失败只记录，不返回错误。
func (s *Service) embedInto(ctx context.Context, memoryID, content string, res *Result) {
	if s.embedder == nil {
		return
	}
	ectx, cancel := context.WithTimeout(ctx, s.embedTimeout)
	defer cancel()

	vecs, err := embed.EmbedDocuments(ectx, s.embedder, []string{content})
	if err != nil {
		res.EmbeddingFailed = true
		res.EmbeddingErr = err.Error()
		return
	}
	if len(vecs) != 1 || len(vecs[0]) == 0 {
		res.EmbeddingFailed = true
		res.EmbeddingErr = fmt.Sprintf("unexpected embedding shape: %d vectors", len(vecs))
		return
	}
	if err := s.store.PutVector(ctx, memoryID, s.embedder.Model(), vecs[0]); err != nil {
		res.EmbeddingFailed = true
		res.EmbeddingErr = err.Error()
		return
	}
	res.Embedded = true
}
