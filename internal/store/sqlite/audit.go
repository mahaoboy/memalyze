package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/id"
	"github.com/mahaoboy/memalyze/internal/store"
)

// 编译期断言：审计接口也必须实现。
var _ store.AuditStore = (*Store)(nil)

// AppendAudit 追加一条审计记录。
//
// 只做 INSERT：不 UPDATE、不 UPSERT。数据库层还有 audit_no_update 触发器兜底，
// 保证「谁读过哪条记忆」这条证据链不可被事后篡改。
func (s *Store) AppendAudit(ctx context.Context, e store.AuditEntry) error {
	if strings.TrimSpace(e.TenantID) == "" {
		return fmt.Errorf("%w: audit tenant_id required", store.ErrInvalid)
	}
	if !e.Action.Valid() {
		return fmt.Errorf("%w: audit action %q", store.ErrInvalid, e.Action)
	}
	if e.ID == "" {
		e.ID = id.New()
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log
		 (id, ts, actor, tenant_id, scope, action, memory_id, query_hash, query_len,
		  result_count, degraded, duplicate, redacted, dry_run, detail)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, ms(e.Time), nullable(e.Actor), e.TenantID, nullable(e.Scope), string(e.Action),
		nullable(e.MemoryID), nullable(e.QueryHash), e.QueryLen, e.ResultCount,
		boolInt(e.Degraded), boolInt(e.Duplicate), e.Redacted, boolInt(e.DryRun), nullable(e.Detail))
	if err != nil {
		return fmt.Errorf("sqlite: append audit: %w", err)
	}
	return nil
}

// QueryAudit 按时间升序返回审计（导出用）。
func (s *Store) QueryAudit(ctx context.Context, f store.AuditFilter) ([]store.AuditEntry, error) {
	var (
		b    strings.Builder
		args []any
	)
	b.WriteString(`SELECT id, ts, actor, tenant_id, scope, action, memory_id, query_hash,
		query_len, result_count, degraded, duplicate, redacted, dry_run, detail
		FROM audit_log WHERE 1 = 1`)
	if f.TenantID != "" {
		b.WriteString(` AND tenant_id = ?`)
		args = append(args, f.TenantID)
	}
	if !f.From.IsZero() {
		b.WriteString(` AND ts >= ?`)
		args = append(args, ms(f.From))
	}
	if !f.To.IsZero() {
		b.WriteString(` AND ts <= ?`)
		args = append(args, ms(f.To))
	}
	if len(f.Actions) > 0 {
		b.WriteString(` AND action IN (`)
		for i, a := range f.Actions {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("?")
			args = append(args, string(a))
		}
		b.WriteString(`)`)
	}
	b.WriteString(` ORDER BY ts ASC, id ASC LIMIT ? OFFSET ?`)
	args = append(args, auditLimit(f.Limit), max(f.Offset, 0))

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query audit: %w", err)
	}
	defer rows.Close()

	var out []store.AuditEntry
	for rows.Next() {
		var (
			e                          store.AuditEntry
			ts                         int64
			actor, scope, memID, qhash sql.NullString
			detail                     sql.NullString
			degraded, dup, dryRun      int
			action                     string
		)
		if err := rows.Scan(&e.ID, &ts, &actor, &e.TenantID, &scope, &action, &memID,
			&qhash, &e.QueryLen, &e.ResultCount, &degraded, &dup, &e.Redacted, &dryRun, &detail); err != nil {
			return nil, fmt.Errorf("sqlite: scan audit: %w", err)
		}
		e.Time = fromMs(ts)
		e.Actor = actor.String
		e.Scope = scope.String
		e.MemoryID = memID.String
		e.QueryHash = qhash.String
		e.Detail = detail.String
		e.Action = store.AuditAction(action)
		e.Degraded = degraded != 0
		e.Duplicate = dup != 0
		e.DryRun = dryRun != 0
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: audit rows: %w", err)
	}
	return out, nil
}

func auditLimit(n int) int {
	switch {
	case n <= 0:
		return 1000
	case n > 100000:
		return 100000
	}
	return n
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func fromMs(v int64) time.Time { return time.UnixMilli(v).UTC() }
