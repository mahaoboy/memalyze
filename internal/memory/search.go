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

// DefaultQueryTimeout 是检索侧 embedding 的超时。
//
// 3 秒是硬约束（S7 教训：竞品语义检索能卡 60 秒）。超时立刻降级到关键词，
// 不允许"等模型"把整个请求拖死。
const DefaultQueryTimeout = 3 * time.Second

// 降级原因（写进 SearchResult.DegradeReason，便于日志与后续埋点）。
const (
	// DegradeNoEmbedder：没有配置 embedding，只能用关键词。
	DegradeNoEmbedder = "no_embedder"
	// DegradeTimeout：查询向量化超时。
	DegradeTimeout = "query_timeout"
	// DegradeError：查询向量化或向量检索报错。
	DegradeError = "vector_error"
)

// SearchResult 是一次检索的结果。
type SearchResult struct {
	Hits []types.SearchHit
	// Degraded 表示向量路径不可用、本轮结果只来自关键词。
	Degraded bool
	// DegradeReason 是降级原因（未降级时为空）。
	DegradeReason string
}

// Search 执行一次检索：向量优先，关键词补位。
//
// 合并规则（D-012 的落地，刻意简单）：
//   - 向量命中在前，保持其原始顺序（分数不参与跨路比较 —— bm25 与余弦不可比）
//   - 命中数不足 limit 时用关键词结果补位，按 id 去重
//   - 同一 id 被两路命中时，保留向量那条，并把 "keyword" 合并进 MatchedBy
//   - 向量路不可用（无 embedder / 超时 / 报错）→ Degraded=true，只返回关键词结果
//
// Query 为空时走 Recent（列表语义），不涉及向量，不标记降级。
func (s *Service) Search(ctx context.Context, q types.SearchQuery) (res SearchResult, err error) {
	// 每次检索都留痕（含失败）：这是"谁在何时读了哪条记忆"的唯一来源。
	defer func() { s.auditSearch(ctx, q, res, err) }()

	if s.store == nil {
		return SearchResult{}, fmt.Errorf("%w: nil store", store.ErrInvalid)
	}
	if q.TenantID == "" {
		return SearchResult{}, fmt.Errorf("%w: tenant_id required", store.ErrInvalid)
	}
	if strings.TrimSpace(q.Query) == "" {
		res.Hits, err = s.store.Recent(ctx, q)
		return res, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}

	var vectorHits []types.SearchHit

	vectorErr := s.vectorSearch(ctx, q, &vectorHits)
	if vectorErr != nil {
		res.Degraded = true
		res.DegradeReason = vectorErr.reason
	}

	// 向量命中不足 limit（含为 0）时用关键词补位
	if len(vectorHits) < limit {
		keywordHits, kwErr := s.store.SearchKeyword(ctx, q)
		switch {
		case kwErr == nil:
			vectorHits = mergeHits(vectorHits, keywordHits, limit)
		case len(vectorHits) == 0:
			// 两路都挂了才把错误抛出去（向量有结果时不影响使用）
			return res, kwErr
		}
	}

	res.Hits = vectorHits
	return res, nil
}

type vectorFailure struct {
	reason string
	err    error
}

func (v *vectorFailure) Error() string { return v.reason + ": " + v.err.Error() }
func (v *vectorFailure) Unwrap() error { return v.err }

// vectorSearch 走"查询向量化 + 余弦检索"，把结果写进 dst。
func (s *Service) vectorSearch(ctx context.Context, q types.SearchQuery, dst *[]types.SearchHit) *vectorFailure {
	if s.embedder == nil {
		return &vectorFailure{reason: DegradeNoEmbedder, err: errors.New("no embedder configured")}
	}

	qctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	qvec, err := embed.EmbedQuery(qctx, s.embedder, q.Query)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &vectorFailure{reason: DegradeTimeout, err: err}
		}
		return &vectorFailure{reason: DegradeError, err: err}
	}
	if len(qvec) == 0 {
		return &vectorFailure{reason: DegradeError, err: errors.New("empty query vector")}
	}

	// 检索本身用外层 ctx（内部是本地 SQLite 扫描，不存在慢网络调用）
	hits, err := s.store.SearchVector(ctx, q, s.embedder.Model(), qvec)
	if err != nil {
		return &vectorFailure{reason: DegradeError, err: err}
	}
	*dst = hits
	return nil
}

// mergeHits 按"向量优先 + 关键词补位"合并，按 id 去重并合并 MatchedBy。
func mergeHits(primary, secondary []types.SearchHit, limit int) []types.SearchHit {
	index := make(map[string]int, limit)
	out := make([]types.SearchHit, 0, limit)

	add := func(h types.SearchHit) {
		if len(out) >= limit {
			return
		}
		if i, ok := index[h.Memory.ID]; ok {
			out[i].MatchedBy = unionStrings(out[i].MatchedBy, h.MatchedBy)
			return
		}
		index[h.Memory.ID] = len(out)
		out = append(out, h)
	}
	for _, h := range primary {
		add(h)
	}
	for _, h := range secondary {
		add(h)
	}
	return out
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string{}, a...), b...) {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
