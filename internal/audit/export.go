// Package audit 把审计记录导出成可交给客户/审计员的格式（S15 验收第 4 条）。
//
// 刻意不做 UI、不做分页：一条命令、一个文件，够用就行（明确不做清单见 S15 目标）。
package audit

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
)

// Format 是导出格式。
type Format string

const (
	FormatCSV  Format = "csv"
	FormatJSON Format = "json"
)

// ParseFormat 解析格式名，空值默认 csv。
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "csv":
		return FormatCSV, nil
	case "json":
		return FormatJSON, nil
	}
	return "", fmt.Errorf("audit: unknown format %q (want csv or json)", s)
}

// row 是 JSON 导出的单行结构（字段名与 CSV 表头一致，便于客户对账）。
type row struct {
	Time        string `json:"time"`
	ID          string `json:"id"`
	Action      string `json:"action"`
	Actor       string `json:"actor"`
	TenantID    string `json:"tenant_id"`
	Scope       string `json:"scope,omitempty"`
	MemoryID    string `json:"memory_id,omitempty"`
	QueryHash   string `json:"query_hash,omitempty"`
	QueryLen    int    `json:"query_len"`
	ResultCount int    `json:"result_count"`
	Degraded    bool   `json:"degraded"`
	Duplicate   bool   `json:"duplicate"`
	Redacted    int    `json:"redacted"`
	DryRun      bool   `json:"dry_run"`
	Detail      string `json:"detail,omitempty"`
}

var header = []string{
	"time", "id", "action", "actor", "tenant_id", "scope", "memory_id",
	"query_hash", "query_len", "result_count", "degraded", "duplicate",
	"redacted", "dry_run", "detail",
}

func toRow(e store.AuditEntry) row {
	return row{
		Time:        e.Time.UTC().Format(time.RFC3339),
		ID:          e.ID,
		Action:      string(e.Action),
		Actor:       e.Actor,
		TenantID:    e.TenantID,
		Scope:       e.Scope,
		MemoryID:    e.MemoryID,
		QueryHash:   e.QueryHash,
		QueryLen:    e.QueryLen,
		ResultCount: e.ResultCount,
		Degraded:    e.Degraded,
		Duplicate:   e.Duplicate,
		Redacted:    e.Redacted,
		DryRun:      e.DryRun,
		Detail:      e.Detail,
	}
}

// Write 把 entries 写到 w。
func Write(w io.Writer, entries []store.AuditEntry, format Format) error {
	switch format {
	case FormatJSON:
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, toRow(e))
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	case FormatCSV, "":
		cw := csv.NewWriter(w)
		if err := cw.Write(header); err != nil {
			return fmt.Errorf("audit: write header: %w", err)
		}
		for _, e := range entries {
			r := toRow(e)
			rec := []string{
				r.Time, r.ID, r.Action, r.Actor, r.TenantID, r.Scope, r.MemoryID,
				r.QueryHash, strconv.Itoa(r.QueryLen), strconv.Itoa(r.ResultCount),
				strconv.FormatBool(r.Degraded), strconv.FormatBool(r.Duplicate),
				strconv.Itoa(r.Redacted), strconv.FormatBool(r.DryRun), r.Detail,
			}
			if err := cw.Write(rec); err != nil {
				return fmt.Errorf("audit: write row: %w", err)
			}
		}
		cw.Flush()
		return cw.Error()
	}
	return fmt.Errorf("audit: unknown format %q", format)
}
