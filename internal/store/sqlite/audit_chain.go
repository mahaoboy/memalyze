package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mahaoboy/memalyze/internal/store"
)

const auditChainDomain = "memalyze-audit-event-v1\x00"

const (
	legacyAuditColumns = `id, ts, actor, tenant_id, scope, action, memory_id, query_hash,
		query_len, result_count, degraded, duplicate, redacted, dry_run, detail`
	auditColumns = `seq, id, ts, actor, tenant_id, scope, action, memory_id, query_hash,
		query_len, result_count, degraded, duplicate, redacted, dry_run, detail, prev_hash, hash`
)

// canonicalAuditEvent 使用结构体 JSON 的固定字段顺序与转义规则，避免手工拼接的歧义。
// 空字符串与 SQL NULL 统一编码为 null，因为写入路径原本就把空可选字段存成 NULL。
type canonicalAuditEvent struct {
	Version     int     `json:"version"`
	Seq         int64   `json:"seq"`
	ID          *string `json:"id"`
	TimestampMS int64   `json:"ts"`
	Actor       *string `json:"actor"`
	TenantID    string  `json:"tenant_id"`
	Scope       *string `json:"scope"`
	Action      string  `json:"action"`
	MemoryID    *string `json:"memory_id"`
	QueryHash   *string `json:"query_hash"`
	QueryLen    int     `json:"query_len"`
	ResultCount int     `json:"result_count"`
	Degraded    bool    `json:"degraded"`
	Duplicate   bool    `json:"duplicate"`
	Redacted    int     `json:"redacted"`
	DryRun      bool    `json:"dry_run"`
	Detail      *string `json:"detail"`
	PrevHash    string  `json:"prev_hash"`
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func canonicalAuditID(e store.AuditEntry) *string {
	if e.IDNull {
		return nil
	}
	v := e.ID
	return &v
}

func hashAuditEntry(e store.AuditEntry) string {
	canonical := canonicalAuditEvent{
		Version:     store.AuditChainVersion,
		Seq:         e.Seq,
		ID:          canonicalAuditID(e),
		TimestampMS: ms(e.Time),
		Actor:       optionalString(e.Actor),
		TenantID:    e.TenantID,
		Scope:       optionalString(e.Scope),
		Action:      string(e.Action),
		MemoryID:    optionalString(e.MemoryID),
		QueryHash:   optionalString(e.QueryHash),
		QueryLen:    e.QueryLen,
		ResultCount: e.ResultCount,
		Degraded:    e.Degraded,
		Duplicate:   e.Duplicate,
		Redacted:    e.Redacted,
		DryRun:      e.DryRun,
		Detail:      optionalString(e.Detail),
		PrevHash:    e.PrevHash,
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		// 该结构只含 JSON 原生类型，Marshal 不会失败；若未来改动破坏此约定，宁可 panic
		// 也不允许把未哈希的事件写入审计链。
		panic(fmt.Sprintf("sqlite: marshal canonical audit event: %v", err))
	}
	h := sha256.New()
	_, _ = io.WriteString(h, auditChainDomain)
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

type auditRecord struct {
	entry                       store.AuditEntry
	ts                          int64
	id                          sql.NullString
	actor, scope, memoryID      sql.NullString
	queryHash, detail           sql.NullString
	action                      string
	degraded, duplicate, dryRun int
}

func (r *auditRecord) targets(withChain bool) []any {
	targets := make([]any, 0, 18)
	if withChain {
		targets = append(targets, &r.entry.Seq)
	}
	targets = append(targets,
		&r.id, &r.ts, &r.actor, &r.entry.TenantID, &r.scope, &r.action,
		&r.memoryID, &r.queryHash, &r.entry.QueryLen, &r.entry.ResultCount,
		&r.degraded, &r.duplicate, &r.entry.Redacted, &r.dryRun, &r.detail,
	)
	if withChain {
		targets = append(targets, &r.entry.PrevHash, &r.entry.Hash)
	}
	return targets
}

func (r *auditRecord) scannedEntry() store.AuditEntry {
	e := r.entry
	e.ID = r.id.String
	e.IDNull = !r.id.Valid
	e.Time = fromMs(r.ts)
	e.Actor = r.actor.String
	e.Scope = r.scope.String
	e.MemoryID = r.memoryID.String
	e.QueryHash = r.queryHash.String
	e.Detail = r.detail.String
	e.Action = store.AuditAction(r.action)
	e.Degraded = r.degraded != 0
	e.Duplicate = r.duplicate != 0
	e.DryRun = r.dryRun != 0
	return e
}

func scanAudit(row scanner, withChain bool) (store.AuditEntry, error) {
	var r auditRecord
	if err := row.Scan(r.targets(withChain)...); err != nil {
		return store.AuditEntry{}, err
	}
	return r.scannedEntry(), nil
}

type contextExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertAuditEvent(ctx context.Context, exec contextExecer, e store.AuditEntry) error {
	var eventID any = e.ID
	if e.IDNull {
		eventID = nil
	}
	_, err := exec.ExecContext(ctx,
		`INSERT INTO audit_log
		 (seq, id, ts, actor, tenant_id, scope, action, memory_id, query_hash,
		  query_len, result_count, degraded, duplicate, redacted, dry_run, detail, prev_hash, hash)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.Seq, eventID, ms(e.Time), nullable(e.Actor), e.TenantID, nullable(e.Scope), string(e.Action),
		nullable(e.MemoryID), nullable(e.QueryHash), e.QueryLen, e.ResultCount,
		boolInt(e.Degraded), boolInt(e.Duplicate), e.Redacted, boolInt(e.DryRun), nullable(e.Detail),
		e.PrevHash, e.Hash)
	return err
}

func migrateAuditChain(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range auditChainPreDDL {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("prepare audit chain: %w", err)
		}
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT `+legacyAuditColumns+` FROM audit_log_v2 ORDER BY ts ASC, id ASC, rowid ASC`)
	if err != nil {
		return fmt.Errorf("read legacy audit: %w", err)
	}
	var entries []store.AuditEntry
	for rows.Next() {
		e, err := scanAudit(rows, false)
		if err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy audit: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy audit: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy audit rows: %w", err)
	}

	prev := store.AuditGenesisHash
	for i := range entries {
		e := entries[i]
		e.Seq = int64(i + 1)
		e.PrevHash = prev
		e.Hash = hashAuditEntry(e)
		if err := insertAuditEvent(ctx, tx, e); err != nil {
			return fmt.Errorf("backfill audit seq %d: %w", e.Seq, err)
		}
		prev = e.Hash
	}

	for _, stmt := range auditChainPostDDL {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("finish audit chain: %w", err)
		}
	}
	return nil
}

// VerifyAudit 复算完整链。expected 非空时还要求事件数、末尾序号和链头完全一致；
// 没有这个外部信任锚时，删除链尾后剩余前缀仍可能是一条内部有效的链。
func (s *Store) VerifyAudit(ctx context.Context, expected *store.AuditCheckpoint) (store.AuditVerification, error) {
	return verifyAudit(ctx, s.db, expected)
}

func verifyAudit(ctx context.Context, q contextQueryer, expected *store.AuditCheckpoint) (store.AuditVerification, error) {
	v := store.AuditVerification{
		OK:               true,
		Algorithm:        store.AuditChainAlgorithm,
		Head:             store.AuditGenesisHash,
		CheckpointStatus: "not_provided",
	}
	var reasons []string
	fail := func(seq int64, reason string) {
		v.OK = false
		if v.FirstInvalidSeq == 0 && seq > 0 {
			v.FirstInvalidSeq = seq
		}
		for _, existing := range reasons {
			if existing == reason {
				return
			}
		}
		reasons = append(reasons, reason)
	}

	rows, err := q.QueryContext(ctx, `SELECT `+auditColumns+` FROM audit_log ORDER BY seq ASC`)
	if err != nil {
		return store.AuditVerification{}, fmt.Errorf("sqlite: query audit chain: %w", err)
	}
	defer rows.Close()

	prev := store.AuditGenesisHash
	for rows.Next() {
		e, err := scanAudit(rows, true)
		if err != nil {
			return store.AuditVerification{}, fmt.Errorf("sqlite: scan audit chain: %w", err)
		}
		v.EventCount++
		v.LastSeq = e.Seq
		v.Head = e.Hash
		if e.Seq != v.EventCount {
			// 报告预期的序号：删除 seq=2 后，第一条异常幸存行是 seq=3，但缺口从 2 开始。
			fail(v.EventCount, "sequence_mismatch")
		}
		if e.PrevHash != prev {
			fail(e.Seq, "previous_hash_mismatch")
		}
		if got := hashAuditEntry(e); got != e.Hash {
			fail(e.Seq, "hash_mismatch")
		}
		prev = e.Hash
	}
	if err := rows.Err(); err != nil {
		return store.AuditVerification{}, fmt.Errorf("sqlite: iterate audit chain: %w", err)
	}

	if expected != nil {
		expectedCopy := *expected
		v.ExpectedCheckpoint = &expectedCopy
		v.CheckpointStatus = "matched"
		if expected.SchemaVersion != store.AuditChainVersion ||
			expected.Algorithm != store.AuditChainAlgorithm ||
			expected.EventCount != v.EventCount || expected.LastSeq != v.LastSeq || expected.Head != v.Head {
			v.CheckpointStatus = "mismatch"
			fail(0, "checkpoint_mismatch")
		}
	}
	v.Reason = strings.Join(reasons, ";")
	return v, nil
}

// AuditSnapshot 在同一个 SQLite 读快照中完成全链校验与筛选聚合输入读取。
// LIMIT/OFFSET 分页不能跨独立语句使用：并发插入旧时间戳会移动 OFFSET。
func (s *Store) AuditSnapshot(ctx context.Context, filter store.AuditFilter, expected *store.AuditCheckpoint) ([]store.AuditEntry, store.AuditVerification, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, store.AuditVerification{}, fmt.Errorf("sqlite: begin audit snapshot: %w", err)
	}
	defer tx.Rollback()

	verification, err := verifyAudit(ctx, tx, expected)
	if err != nil {
		return nil, store.AuditVerification{}, err
	}
	var entries []store.AuditEntry
	for offset := 0; ; offset += 1000 {
		pageFilter := filter
		pageFilter.Limit = 1000
		pageFilter.Offset = offset
		page, err := queryAudit(ctx, tx, pageFilter)
		if err != nil {
			return nil, verification, err
		}
		entries = append(entries, page...)
		if len(page) < 1000 {
			break
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, verification, fmt.Errorf("sqlite: finish audit snapshot: %w", err)
	}
	return entries, verification, nil
}
