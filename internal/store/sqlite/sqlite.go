// Package sqlite 是 store.Store 的 SQLite 实现（CGO-free，modernc.org/sqlite）。
//
// 关键设计（均已实测，见 04-技术路线/数据模型 与 检索验证-S8）：
//   - 向量存 BLOB（float32 小端），检索用暴力余弦
//   - FTS5 索引的是「二元组文本」而不是原文（unicode61 对中文不可用）
//   - 软删除只改 status；去重靠 (tenant_id, scope, content_hash) 的部分唯一索引
package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/embed"
	"github.com/mahaoboy/memalyze/internal/id"
	"github.com/mahaoboy/memalyze/internal/search"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/pkg/types"

	_ "modernc.org/sqlite"
)

// schemaVersion 记在 PRAGMA user_version 里，迁移时按版本递增。
const schemaVersion = 1

// Store 实现 store.Store。
type Store struct {
	db *sql.DB
}

// 编译期断言：确保实现没漏方法。
var _ store.Store = (*Store)(nil)

// Open 打开（或创建）一个 SQLite 库并执行迁移。
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	// v0.1 单进程低频写入：单连接避免 SQLITE_BUSY，也让 :memory: 语义确定。
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite: %s: %w", p, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("sqlite: read user_version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("sqlite: database schema v%d is newer than supported v%d", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin migration: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range schemaV1 {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite: migrate: %w", err)
		}
	}
	// PRAGMA 不支持占位符，这里版本号是常量，拼接安全。
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("sqlite: set user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit migration: %w", err)
	}
	return nil
}

// Add 写入一条记忆（含去重）。
func (s *Store) Add(ctx context.Context, m *types.Memory) (string, error) {
	if m == nil || strings.TrimSpace(m.Content) == "" {
		return "", fmt.Errorf("%w: empty content", store.ErrInvalid)
	}
	normalize(m)

	hash := contentHash(m.Content)
	ftsText := search.BigramText(m.Content)

	// 先查重：命中就返回已有 id（去重是产品行为，不算错误处理路径）
	var existing string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM memories WHERE tenant_id = ? AND scope = ? AND content_hash = ? AND status = 'active'`,
		m.TenantID, m.Scope, hash).Scan(&existing)
	switch {
	case err == nil:
		return existing, store.ErrDuplicate
	case !errors.Is(err, sql.ErrNoRows):
		return "", fmt.Errorf("sqlite: dedup lookup: %w", err)
	}

	metadata, err := marshalMetadata(m.Metadata)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO memories
		 (id, tenant_id, scope, content, fts_text, content_hash, kind, source, metadata,
		  status, created_at, updated_at, expires_at, last_access_at, access_count)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.TenantID, m.Scope, m.Content, ftsText, hash, string(m.Kind), nullable(m.Source),
		metadata, string(m.Status), ms(m.CreatedAt), ms(m.UpdatedAt),
		nullTimeMs(m.ExpiresAt), nullTimeMs(m.LastAccessAt), m.AccessCount)
	if err != nil {
		// 并发下唯一索引仍可能撞：回查一次，保持幂等语义
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			if id, qerr := s.findActiveByHash(ctx, m.TenantID, m.Scope, hash); qerr == nil {
				return id, store.ErrDuplicate
			}
		}
		return "", fmt.Errorf("sqlite: insert memory: %w", err)
	}
	return m.ID, nil
}

// Get 按 id 读取（校验租户）。
func (s *Store) Get(ctx context.Context, tenantID, memoryID string) (*types.Memory, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+memoryColumns+` FROM memories WHERE tenant_id = ? AND id = ?`, tenantID, memoryID)
	m, err := scanMemory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get memory: %w", err)
	}
	return m, nil
}

// SoftDelete 软删除（幂等：已删除再删返回 nil）。
func (s *Store) SoftDelete(ctx context.Context, tenantID, memoryID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET status = ?, updated_at = ? WHERE tenant_id = ? AND id = ? AND status <> ?`,
		string(types.StatusDeleted), ms(time.Now()), tenantID, memoryID, string(types.StatusDeleted))
	if err != nil {
		return fmt.Errorf("sqlite: soft delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: soft delete rows: %w", err)
	}
	if n > 0 {
		return nil
	}
	// 没更新到：要么不存在，要么本来就删了 —— 区分开
	if _, err := s.Get(ctx, tenantID, memoryID); err != nil {
		return err
	}
	return nil
}

// PutVector 写入/覆盖向量。
func (s *Store) PutVector(ctx context.Context, memoryID, model string, vec []float32) error {
	if memoryID == "" || model == "" || len(vec) == 0 {
		return fmt.Errorf("%w: memory_id/model/vector required", store.ErrInvalid)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO embeddings (memory_id, model, dim, vector, created_at) VALUES (?,?,?,?,?)
		 ON CONFLICT(memory_id, model) DO UPDATE SET dim = excluded.dim, vector = excluded.vector, created_at = excluded.created_at`,
		memoryID, model, len(vec), embed.Encode(vec), ms(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlite: put vector: %w", err)
	}
	return nil
}

// SearchKeyword 用 FTS5 二元组索引检索。
func (s *Store) SearchKeyword(ctx context.Context, q types.SearchQuery) ([]types.SearchHit, error) {
	if q.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id required", store.ErrInvalid)
	}
	expr := search.MatchExpr(q.Query)
	if expr == "" {
		return nil, nil
	}
	where, args := buildFilter(q, time.Now())
	query := `SELECT m.id, m.tenant_id, m.scope, m.content, m.kind, m.source, m.metadata,
	                 m.status, m.created_at, m.updated_at, m.expires_at, m.last_access_at, m.access_count,
	                 bm25(memories_fts) AS rank
	          FROM memories_fts JOIN memories m ON m.rowid = memories_fts.rowid
	          WHERE memories_fts MATCH ?` + where + ` ORDER BY rank LIMIT ?`
	args = append([]any{expr}, args...)
	args = append(args, limitOf(q))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: keyword search: %w", err)
	}
	defer rows.Close()

	var hits []types.SearchHit
	for rows.Next() {
		m, rank, err := scanMemoryWithRank(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan keyword hit: %w", err)
		}
		hits = append(hits, types.SearchHit{Memory: *m, Score: -rank, MatchedBy: []string{"keyword"}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: keyword rows: %w", err)
	}
	return hits, nil
}

// SearchVector 用暴力余弦检索（候选集先按租户/scope/状态过滤）。
func (s *Store) SearchVector(ctx context.Context, q types.SearchQuery, model string, vec []float32) ([]types.SearchHit, error) {
	if q.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id required", store.ErrInvalid)
	}
	if model == "" || len(vec) == 0 {
		return nil, fmt.Errorf("%w: model and query vector required", store.ErrInvalid)
	}
	where, args := buildFilter(q, time.Now())
	query := `SELECT m.id, m.tenant_id, m.scope, m.content, m.kind, m.source, m.metadata,
	                 m.status, m.created_at, m.updated_at, m.expires_at, m.last_access_at, m.access_count,
	                 e.vector
	          FROM memories m JOIN embeddings e ON e.memory_id = m.id
	          WHERE e.model = ?` + where
	args = append([]any{model}, args...)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: vector search: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		mem  types.Memory
		blob []byte
	}
	var cands []candidate
	for rows.Next() {
		cols := append(memoryScanTargets(), new([]byte))
		if err := rows.Scan(cols...); err != nil {
			return nil, fmt.Errorf("sqlite: scan vector candidate: %w", err)
		}
		m := memoryFromTargets(cols)
		blob := *(cols[len(cols)-1].(*[]byte))
		cands = append(cands, candidate{mem: *m, blob: blob})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: vector rows: %w", err)
	}

	hits := make([]types.SearchHit, 0, len(cands))
	for _, c := range cands {
		stored, err := embed.Decode(c.blob)
		if err != nil {
			continue // 脏数据跳过而不是整体失败
		}
		score, err := embed.Cosine(vec, stored)
		if err != nil {
			continue // 维度不一致（换过模型）→ 跳过
		}
		hits = append(hits, types.SearchHit{Memory: c.mem, Score: score, MatchedBy: []string{"vector"}})
	}
	sortHits(hits)
	if len(hits) > limitOf(q) {
		hits = hits[:limitOf(q)]
	}
	return hits, nil
}

// Recent 按创建时间倒序返回（空查询时的列表语义）。
func (s *Store) Recent(ctx context.Context, q types.SearchQuery) ([]types.SearchHit, error) {
	if q.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id required", store.ErrInvalid)
	}
	where, args := buildFilter(q, time.Now())
	query := `SELECT ` + memoryColumns + ` FROM memories m WHERE 1 = 1` + where +
		` ORDER BY m.created_at DESC, m.id DESC LIMIT ?`
	args = append(args, limitOf(q))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: recent: %w", err)
	}
	defer rows.Close()

	var hits []types.SearchHit
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan recent: %w", err)
		}
		hits = append(hits, types.SearchHit{Memory: *m, MatchedBy: []string{"recent"}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: recent rows: %w", err)
	}
	return hits, nil
}

// ---- 内部辅助 ----

const memoryColumns = `id, tenant_id, scope, content, kind, source, metadata, status,
	created_at, updated_at, expires_at, last_access_at, access_count`

// buildFilter 生成公共过滤条件（租户 / scope / 状态 / TTL / kind）。
func buildFilter(q types.SearchQuery, now time.Time) (string, []any) {
	var b strings.Builder
	args := []any{q.TenantID}
	b.WriteString(` AND m.tenant_id = ?`)

	if q.Scope != "" {
		if q.ScopePrefix {
			b.WriteString(` AND (m.scope = ? OR m.scope LIKE ? || ':%')`)
			args = append(args, q.Scope, q.Scope)
		} else {
			b.WriteString(` AND m.scope = ?`)
			args = append(args, q.Scope)
		}
	}
	b.WriteString(` AND m.status = 'active'`)
	if !q.IncludeExpired {
		b.WriteString(` AND (m.expires_at IS NULL OR m.expires_at > ?)`)
		args = append(args, ms(now))
	}
	if len(q.Kinds) > 0 {
		b.WriteString(` AND m.kind IN (`)
		for i, k := range q.Kinds {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("?")
			args = append(args, string(k))
		}
		b.WriteString(`)`)
	}
	return b.String(), args
}

func limitOf(q types.SearchQuery) int {
	if q.Limit <= 0 {
		return 10
	}
	if q.Limit > 100 {
		return 100
	}
	return q.Limit
}

func (s *Store) findActiveByHash(ctx context.Context, tenantID, scope, hash string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM memories WHERE tenant_id = ? AND scope = ? AND content_hash = ? AND status = 'active'`,
		tenantID, scope, hash).Scan(&id)
	return id, err
}

func normalize(m *types.Memory) {
	if m.TenantID == "" {
		m.TenantID = "default"
	}
	if m.Scope == "" {
		m.Scope = "default"
	}
	if m.Kind == "" || !m.Kind.Valid() {
		m.Kind = types.KindNote
	}
	if m.Status == "" || !m.Status.Valid() {
		m.Status = types.StatusActive
	}
	now := time.Now()
	if m.ID == "" {
		m.ID = id.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	m.Content = strings.TrimSpace(m.Content)
}

// contentHash 对规范化后的内容取 sha256（去重键）。
func contentHash(content string) string {
	normalized := strings.Join(strings.Fields(content), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func marshalMetadata(md map[string]any) (sql.NullString, error) {
	if len(md) == 0 {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(md)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("%w: metadata: %v", store.ErrInvalid, err)
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}

func nullable(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func ms(t time.Time) int64 { return t.UnixMilli() }

func nullTimeMs(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

// memoryScanTargets 返回一组扫描目标，供 Scan 与 memoryFromTargets 共用。
func memoryScanTargets() []any {
	var (
		id, tenantID, scope, content, kind string
		source, metadata, status           sql.NullString
		createdAt, updatedAt               int64
		expiresAt, lastAccessAt            sql.NullInt64
		accessCount                        int64
	)
	return []any{&id, &tenantID, &scope, &content, &kind, &source, &metadata, &status,
		&createdAt, &updatedAt, &expiresAt, &lastAccessAt, &accessCount}
}

func memoryFromTargets(targets []any) *types.Memory {
	m := &types.Memory{
		ID:          *(targets[0].(*string)),
		TenantID:    *(targets[1].(*string)),
		Scope:       *(targets[2].(*string)),
		Content:     *(targets[3].(*string)),
		Kind:        types.Kind(*(targets[4].(*string))),
		Status:      types.Status(targets[7].(*sql.NullString).String),
		CreatedAt:   time.UnixMilli(*(targets[8].(*int64))).UTC(),
		UpdatedAt:   time.UnixMilli(*(targets[9].(*int64))).UTC(),
		AccessCount: *(targets[12].(*int64)),
	}
	if src := targets[5].(*sql.NullString); src.Valid {
		m.Source = src.String
	}
	if md := targets[6].(*sql.NullString); md.Valid {
		var raw map[string]any
		if json.Unmarshal([]byte(md.String), &raw) == nil {
			m.Metadata = raw
		}
	}
	if exp := targets[10].(*sql.NullInt64); exp.Valid {
		t := time.UnixMilli(exp.Int64).UTC()
		m.ExpiresAt = &t
	}
	if la := targets[11].(*sql.NullInt64); la.Valid {
		t := time.UnixMilli(la.Int64).UTC()
		m.LastAccessAt = &t
	}
	return m
}

// scanner 同时接受 *sql.Row 与 *sql.Rows。
type scanner interface {
	Scan(dest ...any) error
}

func scanMemory(row scanner) (*types.Memory, error) {
	targets := memoryScanTargets()
	if err := row.Scan(targets...); err != nil {
		return nil, err
	}
	return memoryFromTargets(targets), nil
}

func scanMemoryWithRank(rows *sql.Rows) (*types.Memory, float64, error) {
	targets := memoryScanTargets()
	var rank float64
	targets = append(targets, &rank)
	if err := rows.Scan(targets...); err != nil {
		return nil, 0, err
	}
	return memoryFromTargets(targets), rank, nil
}

func sortHits(hits []types.SearchHit) {
	// 插入排序：命中量小（<= 几千），可读性优先
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
}
