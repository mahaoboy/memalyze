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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/embed"
	"github.com/mahaoboy/memalyze/internal/govern"
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
	// Redacted 表示本次写入命中了脱敏规则（内容可能已被改写）。
	Redacted bool
	// RedactionCount 是命中次数；Redactions 是命中的规则名（不含原文）。
	RedactionCount int
	Redactions     []string
	// DryRun 表示只统计未改写（dry-run 模式）。
	DryRun bool
}

// Service 实现写入规则。
type Service struct {
	store         store.Store
	embedder      embed.Embedder
	embedTimeout  time.Duration
	queryTimeout  time.Duration
	warmupTimeout time.Duration
	now           func() time.Time

	redactor        *govern.Redactor
	redactMode      govern.Mode
	auditor         store.AuditStore
	actor           string
	auditQueryPlain bool
	logf            func(format string, args ...any)
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

// WithRedactor 打开写入前脱敏。mode 决定"真的改"还是"只统计"。
func WithRedactor(r *govern.Redactor, mode govern.Mode) Option {
	return func(s *Service) {
		s.redactor = r
		s.redactMode = mode
	}
}

// WithAudit 打开审计。actor 会写进每条记录；plainQuery=true 时检索原文进 Detail（默认只存 hash）。
func WithAudit(a store.AuditStore, actor string, plainQuery bool) Option {
	return func(s *Service) {
		s.auditor = a
		s.actor = actor
		s.auditQueryPlain = plainQuery
	}
}

// WithLogger 注入日志函数（默认写 stderr；stdout 是 MCP 协议通道，不能用）。
func WithLogger(f func(format string, args ...any)) Option {
	return func(s *Service) {
		if f != nil {
			s.logf = f
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
		redactMode:    govern.ModeOff,
		logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		},
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
func (s *Service) Add(ctx context.Context, in Input) (res Result, err error) {
	if s.store == nil {
		return Result{}, fmt.Errorf("%w: nil store", store.ErrInvalid)
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return Result{}, fmt.Errorf("%w: empty content", store.ErrInvalid)
	}

	// 脱敏必须在去重与 embedding 之前（S15 验收第 1 条）：
	// 一旦原文进了 content_hash / fts_text / 向量，就再也擦不干净了。
	if s.redactor != nil && s.redactMode != govern.ModeOff {
		redacted, hits := s.redactor.Redact(content)
		if len(hits) > 0 {
			res.Redacted = true
			res.RedactionCount = govern.Total(hits)
			res.Redactions = govern.HitNames(hits)
			res.DryRun = s.redactMode == govern.ModeDryRun
			if s.redactMode == govern.ModeEnforce {
				content = redacted
			}
		}
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

	defer func() {
		s.audit(ctx, store.AuditEntry{
			TenantID:    in.TenantID,
			Scope:       in.Scope,
			Action:      store.AuditAdd,
			MemoryID:    res.ID,
			ResultCount: resultCount(res.ID != ""),
			Duplicate:   res.Duplicate,
			Redacted:    res.RedactionCount,
			DryRun:      res.DryRun,
			Detail:      errDetail(err),
		})
	}()

	id, err := s.store.Add(ctx, m)
	res.ID = id
	switch {
	case errors.Is(err, store.ErrDuplicate):
		res.Duplicate = true
		err = nil
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
func (s *Service) Delete(ctx context.Context, tenantID, memoryID string) (err error) {
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
	defer func() {
		s.audit(ctx, store.AuditEntry{
			TenantID:    tenantID,
			Action:      store.AuditDelete,
			MemoryID:    memoryID,
			ResultCount: resultCount(err == nil),
			Detail:      errDetail(err),
		})
	}()
	return s.store.SoftDelete(ctx, tenantID, memoryID)
}

// Reembed 重新计算某条记忆的向量（换模型、或上次 embedding 失败时补算）。
//
// 与 Add 不同：这是显式操作，失败会返回错误（调用方需要知道没成功）。
func (s *Service) Reembed(ctx context.Context, tenantID, memoryID string) (res Result, err error) {
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

	res.ID = memoryID
	defer func() {
		s.audit(ctx, store.AuditEntry{
			TenantID:    tenantID,
			Action:      store.AuditReembed,
			MemoryID:    memoryID,
			ResultCount: resultCount(err == nil),
			Detail:      errDetail(err),
		})
	}()
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

// ---- 审计辅助 ----

// auditTimeout 是审计写入的上限：审计失败不能拖慢主流程，也不能因为
// 调用方 ctx 已超时就把证据丢掉（所以用 WithoutCancel 另起一个短超时）。
const auditTimeout = 2 * time.Second

func (s *Service) audit(ctx context.Context, e store.AuditEntry) {
	if s.auditor == nil {
		return
	}
	if e.Action == "" || !e.Action.Valid() {
		s.logf("memalyze: skip audit with invalid action %q", e.Action)
		return
	}
	if e.Actor == "" {
		e.Actor = s.actor
	}
	if e.TenantID == "" {
		s.logf("memalyze: skip audit with empty tenant (action=%s)", e.Action)
		return
	}
	if e.Time.IsZero() {
		e.Time = s.now()
	}
	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditTimeout)
	defer cancel()
	if err := s.auditor.AppendAudit(actx, e); err != nil {
		s.logf("memalyze: audit append failed (action=%s): %v", e.Action, err)
	}
}

// auditSearch 记录一次检索。原文默认不入库，只存 SHA-256 + 长度。
func (s *Service) auditSearch(ctx context.Context, q types.SearchQuery, res SearchResult, err error) {
	e := store.AuditEntry{
		TenantID:    q.TenantID,
		Scope:       q.Scope,
		Action:      store.AuditSearch,
		ResultCount: len(res.Hits),
		Degraded:    res.Degraded,
		Detail:      errDetail(err),
	}
	hash, n := fingerprint(q.Query)
	e.QueryHash, e.QueryLen = hash, n
	if q.Query == "" {
		e.Detail = appendDetail(e.Detail, "list=recent")
	}
	if s.auditQueryPlain && q.Query != "" {
		e.Detail = appendDetail(e.Detail, "query="+q.Query)
	}
	if res.Degraded {
		e.Detail = appendDetail(e.Detail, "degrade="+res.DegradeReason)
	}
	s.audit(ctx, e)
}

func fingerprint(q string) (string, int) {
	if q == "" {
		return "", 0
	}
	sum := sha256.Sum256([]byte(q))
	return hex.EncodeToString(sum[:]), len(q)
}

func errDetail(err error) string {
	if err == nil {
		return ""
	}
	return "error=" + err.Error()
}

func appendDetail(detail, add string) string {
	if add == "" {
		return detail
	}
	if detail == "" {
		return add
	}
	return detail + "; " + add
}

func resultCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
