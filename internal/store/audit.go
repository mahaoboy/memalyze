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
	// Seq 是从 1 开始的链序号；PrevHash/Hash 由存储层生成，调用方不能指定。
	Seq      int64
	PrevHash string
	Hash     string
	ID       string
	// IDNull 仅用于保留 schema v2 中 SQLite 允许的 NULL 主键；新追加事件始终生成非空 ID。
	IDNull   bool `json:"-"`
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

const (
	// AuditChainVersion 是规范化事件与哈希算法的版本。
	AuditChainVersion   = 1
	AuditChainAlgorithm = "sha256-chain-v1"
	// AuditGenesisHash 是第一条事件之前的固定链头。
	AuditGenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"
)

// AuditCheckpoint 是必须独立于数据库保存的信任锚。
// 只有与既有 checkpoint 比较，才能发现攻击者删除整条链尾的情况。
type AuditCheckpoint struct {
	SchemaVersion int    `json:"schema_version"`
	Algorithm     string `json:"algorithm"`
	EventCount    int64  `json:"event_count"`
	LastSeq       int64  `json:"last_seq"`
	Head          string `json:"head"`
}

// AuditVerification 描述全链校验结果；Reason 只含稳定机器可读代码，不含事件内容。
type AuditVerification struct {
	OK                 bool             `json:"ok"`
	Algorithm          string           `json:"algorithm"`
	EventCount         int64            `json:"event_count"`
	LastSeq            int64            `json:"last_seq"`
	Head               string           `json:"head"`
	FirstInvalidSeq    int64            `json:"first_invalid_seq,omitempty"`
	Reason             string           `json:"reason,omitempty"`
	CheckpointStatus   string           `json:"checkpoint_status"`
	ExpectedCheckpoint *AuditCheckpoint `json:"expected_checkpoint,omitempty"`
}

// Checkpoint 返回当前验证结果对应的检查点；失败结果不能作为新的信任锚。
func (v AuditVerification) Checkpoint() (AuditCheckpoint, bool) {
	if !v.OK {
		return AuditCheckpoint{}, false
	}
	return AuditCheckpoint{
		SchemaVersion: AuditChainVersion,
		Algorithm:     v.Algorithm,
		EventCount:    v.EventCount,
		LastSeq:       v.LastSeq,
		Head:          v.Head,
	}, true
}

// AuditVerificationStore 是可选的链校验能力，不强迫其他 Store 实现。
type AuditVerificationStore interface {
	VerifyAudit(ctx context.Context, expected *AuditCheckpoint) (AuditVerification, error)
}

// AuditStore 是审计的存储接口（与 Store 分开：审计可以独立关闭/替换）。
type AuditStore interface {
	// AppendAudit 追加一条审计。实现必须保证只追加、不可改。
	AppendAudit(ctx context.Context, e AuditEntry) error
	// QueryAudit 按时间升序返回，便于导出与逐条核对。
	QueryAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, error)
}
