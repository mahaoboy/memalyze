package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/id"
	"github.com/mahaoboy/memalyze/internal/store"
)

// 编译期断言：审计接口也必须实现。
var _ store.AuditStore = (*Store)(nil)

type contextQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// AppendAudit 在 BEGIN IMMEDIATE 事务中读取链头并追加一条事件。
// 调用方提供的 Seq/PrevHash/Hash 一律忽略，链字段只能由存储层生成。
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
	e.IDNull = false // NULL 仅允许迁移保留，不能由新写入路径制造。
	if e.Time.IsZero() {
		e.Time = time.Now()
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: audit connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("sqlite: begin audit append: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			// 调用方取消也不能留下打开事务；回滚只操作本地连接。
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	var lastSeq int64
	prev := store.AuditGenesisHash
	err = conn.QueryRowContext(ctx, `SELECT seq, hash FROM audit_log ORDER BY seq DESC LIMIT 1`).Scan(&lastSeq, &prev)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		lastSeq = 0
		prev = store.AuditGenesisHash
	case err != nil:
		return fmt.Errorf("sqlite: read audit head: %w", err)
	}

	e.Seq = lastSeq + 1
	e.PrevHash = prev
	e.Hash = hashAuditEntry(e)
	if err := insertAuditEvent(ctx, conn, e); err != nil {
		return fmt.Errorf("sqlite: append audit: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("sqlite: commit audit append: %w", err)
	}
	committed = true
	return nil
}

// QueryAudit 按时间升序返回审计（导出用）。
func (s *Store) QueryAudit(ctx context.Context, f store.AuditFilter) ([]store.AuditEntry, error) {
	return queryAudit(ctx, s.db, f)
}

func queryAudit(ctx context.Context, q contextQueryer, f store.AuditFilter) ([]store.AuditEntry, error) {
	var (
		b    strings.Builder
		args []any
	)
	b.WriteString(`SELECT ` + auditColumns + ` FROM audit_log WHERE 1 = 1`)
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
	b.WriteString(` ORDER BY ts ASC, id ASC, seq ASC LIMIT ? OFFSET ?`)
	args = append(args, auditLimit(f.Limit), max(f.Offset, 0))

	rows, err := q.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query audit: %w", err)
	}
	defer rows.Close()

	var out []store.AuditEntry
	for rows.Next() {
		e, err := scanAudit(rows, true)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan audit: %w", err)
		}
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
