// Package store 定义记忆存储的抽象。
//
// 约定（见 04-技术路线/数据模型 §6）：
//   - 所有读写都必须带 tenant_id，隔离在 Store 层强制，不暴露裸 SQL
//   - v0.1 只实现 SQLite；Postgres 实现后置，接口保持不变
package store

import (
	"context"
	"errors"

	"github.com/mahaoboy/memalyze/pkg/types"
)

var (
	// ErrNotFound 表示记忆不存在（或不属于该租户）。
	ErrNotFound = errors.New("store: memory not found")
	// ErrDuplicate 表示同一租户同一 scope 下已有内容相同的活跃记忆。
	// 返回的 id 是已存在那条记忆的 id，调用方可直接复用。
	ErrDuplicate = errors.New("store: duplicate content in same tenant/scope")
	// ErrInvalid 表示入参不合法（空租户、非法 kind、向量维度为 0 等）。
	ErrInvalid = errors.New("store: invalid input")
)

// Store 是记忆存储接口。
type Store interface {
	// Add 写入一条记忆，返回其 id。
	// 内容重复时返回已存在记忆的 id 和 ErrDuplicate（不算失败）。
	Add(ctx context.Context, m *types.Memory) (string, error)

	// Get 按 id 读取，必须校验租户归属。
	Get(ctx context.Context, tenantID, id string) (*types.Memory, error)

	// SoftDelete 把状态置为 deleted（不物理删，保留可审计）。
	SoftDelete(ctx context.Context, tenantID, id string) error

	// PutVector 写入/覆盖某条记忆在指定模型下的向量。
	PutVector(ctx context.Context, memoryID, model string, vec []float32) error

	// SearchKeyword 走 FTS5 二元组关键词检索。
	SearchKeyword(ctx context.Context, q types.SearchQuery) ([]types.SearchHit, error)

	// SearchVector 走暴力余弦向量检索（v0.1 适用 < 1 万条）。
	SearchVector(ctx context.Context, q types.SearchQuery, model string, vec []float32) ([]types.SearchHit, error)

	// Recent 按创建时间倒序列出记忆（Query 为空时的"列表"语义，不做相关性排序）。
	Recent(ctx context.Context, q types.SearchQuery) ([]types.SearchHit, error)

	// Close 关闭底层连接。
	Close() error
}
