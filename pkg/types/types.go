// Package types 定义 memalyze 的对外类型。
//
// 这些结构是存储层与 MCP 工具层共同的契约（见 04-技术路线/数据模型 §8）。
package types

import "time"

// Kind 是记忆的类别。取值稳定，落库为 TEXT。
type Kind string

// Status 是记忆的生命周期状态。软删除只改状态，不物理删。
type Status string

const (
	KindFact       Kind = "fact"       // 事实：可验证的陈述
	KindPreference Kind = "preference" // 偏好：用户的喜好
	KindEpisode    Kind = "episode"    // 事件：某次交互/经历
	KindNote       Kind = "note"       // 其它笔记（默认）
)

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
	StatusDeleted  Status = "deleted"
)

// Valid 报告 kind 是否合法。
func (k Kind) Valid() bool {
	switch k {
	case KindFact, KindPreference, KindEpisode, KindNote:
		return true
	}
	return false
}

// Valid 报告 status 是否合法。
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusArchived, StatusDeleted:
		return true
	}
	return false
}

// Memory 是一条记忆。
type Memory struct {
	ID           string
	TenantID     string
	Scope        string
	Content      string
	Kind         Kind
	Source       string
	Metadata     map[string]any
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    *time.Time
	LastAccessAt *time.Time
	AccessCount  int64
}

// SearchQuery 是一次检索的输入。
//
// TenantID 必填；Scope 为空表示不限 scope（v0.1 里这仍受 tenant 约束）。
type SearchQuery struct {
	TenantID       string
	Scope          string
	ScopePrefix    bool // true 时 scope 前缀匹配（project:x 命中 project:x:y）
	Query          string
	Limit          int
	Kinds          []Kind
	IncludeExpired bool
}

// SearchHit 是一条命中。
type SearchHit struct {
	Memory    Memory
	Score     float64
	MatchedBy []string // "vector" | "keyword"
}
