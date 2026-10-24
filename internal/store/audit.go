package store

import (
	"context"
	"time"
)

// AuditAction 是审计事件的类型。取值稳定，落库为 TEXT。
type AuditAction string

const (
	AuditAdd     AuditAction = "add"
	AuditSearch  AuditAction = "search"
	AuditDelete  AuditAction = "delete"
	AuditReembed AuditAction = "reembed"
)

// Valid 报告 action 是否合法。
func (a AuditAction) Valid() bool {
	switch a {
	case AuditAdd, AuditSearch, AuditDelete, AuditReembed:
		return true
	}
	return false
}

// AuditEntry 是一条审计记录（append-only，不参与检索）。
//
// 隐私约定（S15 验收第 3 条）：检索原文**默认不落库**，
// 只保存 SHA-256 与长度；要存原文必须显式打开开关。
type AuditEntry struct {
	ID       string
	Time     time.Time
	Actor    string
	TenantID string
	Scope    string
	Action   AuditAction
	// MemoryID 对 add/delete/reembed 必填；search 通常为空。
	MemoryID string
	// QueryHash 是检索语句的 SHA-256（十六进制），QueryLen 是字节长度。
	QueryHash string
	QueryLen  int
	// ResultCount 是本次操作涉及的条数（search=命中数，add/delete=1 或 0）。
	ResultCount int
	// Degraded 表示本轮检索走了关键词降级路径。
	Degraded bool
	// Duplicate 表示 add 命中去重、未写新记录。
	Duplicate bool
	// Redacted 是本次写入被脱敏的命中次数；DryRun 表示只统计未改写。
	Redacted int
	DryRun   bool
	// Detail 是给运维看的补充信息（不得包含记忆原文或检索原文）。
	Detail string
}

// AuditFilter 是审计查询条件。
type AuditFilter struct {
	TenantID string
	From     time.Time // 含
	To       time.Time // 含
	Actions  []AuditAction
	Limit    int
	Offset   int
}

// AuditStore 是审计的存储接口（与 Store 分开：审计可以独立关闭/替换）。
type AuditStore interface {
	// AppendAudit 追加一条审计。实现必须保证只追加、不可改。
	AppendAudit(ctx context.Context, e AuditEntry) error
	// QueryAudit 按时间升序返回，便于导出与逐条核对。
	QueryAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, error)
}
