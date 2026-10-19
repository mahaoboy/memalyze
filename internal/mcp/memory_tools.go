package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mahaoboy/memalyze/internal/memory"
	"github.com/mahaoboy/memalyze/pkg/types"
)

// memoryTools 把 MCP 参数转换成 memory.Service 的入参，并原样搬运结果。
//
// 这里刻意不写去重、TTL、降级等规则：这些属于 memory.Service（见 D-014 / D-015）。
// 工具层只负责「协议类型 <-> 领域类型」的转换，保证以后换 transport 或加工具时
// 业务语义只有一份。
type memoryTools struct {
	svc    *memory.Service
	tenant string
}

func newMemoryTools(svc *memory.Service, tenant string) *memoryTools {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "default"
	}
	return &memoryTools{svc: svc, tenant: tenant}
}

// ---- memory_add ----

type memoryAddArgs struct {
	Content  string `json:"content" jsonschema:"the memory text to store"`
	Scope    string `json:"scope,omitempty" jsonschema:"optional scope, for example project:opc; defaults to 'default'"`
	Kind     string `json:"kind,omitempty" jsonschema:"optional kind: fact, preference, episode, or note; defaults to note"`
	TTLHours int    `json:"ttl_hours,omitempty" jsonschema:"optional time-to-live in hours; 0 means the memory never expires"`
}

type memoryAddOutput struct {
	ID              string `json:"id" jsonschema:"the id of the stored (or already existing) memory"`
	Duplicate       bool   `json:"duplicate" jsonschema:"true when identical content already exists in the same tenant/scope; no new record was written"`
	Embedded        bool   `json:"embedded" jsonschema:"true when a vector was written for this memory"`
	EmbeddingFailed bool   `json:"embedding_failed" jsonschema:"true when the memory was saved but vectorization failed; keyword search still works"`
	EmbeddingError  string `json:"embedding_error,omitempty" jsonschema:"the vectorization error, present only when embedding_failed is true"`
}

func (t *memoryTools) add(ctx context.Context, args memoryAddArgs) (memoryAddOutput, error) {
	if err := t.ready(); err != nil {
		return memoryAddOutput{}, err
	}

	res, err := t.svc.Add(ctx, memory.Input{
		TenantID: t.tenant,
		Scope:    args.Scope,
		Content:  args.Content,
		Kind:     types.Kind(args.Kind),
		TTL:      time.Duration(args.TTLHours) * time.Hour,
	})
	if err != nil {
		return memoryAddOutput{}, err
	}
	return memoryAddOutput{
		ID:              res.ID,
		Duplicate:       res.Duplicate,
		Embedded:        res.Embedded,
		EmbeddingFailed: res.EmbeddingFailed,
		EmbeddingError:  res.EmbeddingErr,
	}, nil
}

// ---- memory_search ----

type memorySearchArgs struct {
	Query          string `json:"query,omitempty" jsonschema:"optional search text; an empty query lists recent memories"`
	Scope          string `json:"scope,omitempty" jsonschema:"optional scope filter; empty means all scopes"`
	Limit          int    `json:"limit,omitempty" jsonschema:"optional maximum number of hits; defaults to 10"`
	IncludeExpired bool   `json:"include_expired,omitempty" jsonschema:"optional; when true, expired memories are included"`
}

type memorySearchHit struct {
	ID        string   `json:"id"`
	Content   string   `json:"content"`
	Score     float64  `json:"score"`
	MatchedBy []string `json:"matched_by" jsonschema:"which retrieval paths matched: vector, keyword, or recent"`
	Kind      string   `json:"kind"`
	Scope     string   `json:"scope"`
	CreatedAt string   `json:"created_at" jsonschema:"RFC 3339 timestamp in UTC"`
}

type memorySearchOutput struct {
	Degraded      bool              `json:"degraded" jsonschema:"true when the vector path was unavailable and results came only from keyword search or the recent list"`
	DegradeReason string            `json:"degrade_reason" jsonschema:"why degradation happened: no_embedder, query_timeout, or vector_error; empty when not degraded"`
	Hits          []memorySearchHit `json:"hits"`
}

func (t *memoryTools) search(ctx context.Context, args memorySearchArgs) (memorySearchOutput, error) {
	if err := t.ready(); err != nil {
		return memorySearchOutput{}, err
	}

	res, err := t.svc.Search(ctx, types.SearchQuery{
		TenantID:       t.tenant,
		Scope:          args.Scope,
		Query:          args.Query,
		Limit:          args.Limit,
		IncludeExpired: args.IncludeExpired,
	})
	if err != nil {
		return memorySearchOutput{}, err
	}

	hits := make([]memorySearchHit, 0, len(res.Hits))
	for _, h := range res.Hits {
		matchedBy := append([]string(nil), h.MatchedBy...)
		if matchedBy == nil {
			matchedBy = []string{}
		}
		hits = append(hits, memorySearchHit{
			ID:        h.Memory.ID,
			Content:   h.Memory.Content,
			Score:     h.Score,
			MatchedBy: matchedBy,
			Kind:      string(h.Memory.Kind),
			Scope:     h.Memory.Scope,
			CreatedAt: h.Memory.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return memorySearchOutput{
		Degraded:      res.Degraded,
		DegradeReason: res.DegradeReason,
		Hits:          hits,
	}, nil
}

// ---- memory_delete ----

type memoryDeleteArgs struct {
	ID string `json:"id" jsonschema:"the id of the memory to delete"`
}

type memoryDeleteOutput struct {
	Deleted bool `json:"deleted" jsonschema:"true when the memory is deleted (already-deleted ids are idempotent)"`
}

func (t *memoryTools) delete(ctx context.Context, args memoryDeleteArgs) (memoryDeleteOutput, error) {
	if err := t.ready(); err != nil {
		return memoryDeleteOutput{}, err
	}
	if err := t.svc.Delete(ctx, t.tenant, args.ID); err != nil {
		return memoryDeleteOutput{}, err
	}
	return memoryDeleteOutput{Deleted: true}, nil
}

// ready 只检查装配是否完整，不承载任何业务规则。
func (t *memoryTools) ready() error {
	if t == nil || t.svc == nil {
		return fmt.Errorf("mcp: memory service is not configured")
	}
	return nil
}

// registerMemoryTools 注册 S12 的三件套。
func registerMemoryTools(server *mcpsdk.Server, svc *memory.Service, tenant string) {
	tools := newMemoryTools(svc, tenant)

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name: "memory_add",
		Description: "Store a memory for later retrieval. Returns duplicate=true when identical content " +
			"already exists (no new record), and embedding_failed=true when the memory was saved but " +
			"vectorization failed (keyword search still works).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args memoryAddArgs) (*mcpsdk.CallToolResult, memoryAddOutput, error) {
		out, err := tools.add(ctx, args)
		if err != nil {
			return nil, memoryAddOutput{}, err
		}
		return nil, out, nil
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name: "memory_search",
		Description: "Search memories with vector search first and keyword fallback. degraded=true means the " +
			"vector path was unavailable; degrade_reason explains why. An empty query lists recent memories.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args memorySearchArgs) (*mcpsdk.CallToolResult, memorySearchOutput, error) {
		out, err := tools.search(ctx, args)
		if err != nil {
			return nil, memorySearchOutput{}, err
		}
		return nil, out, nil
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name: "memory_delete",
		Description: "Soft-delete a memory by id. Already-deleted ids are idempotent; an unknown id returns an " +
			"error instead of pretending to succeed.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args memoryDeleteArgs) (*mcpsdk.CallToolResult, memoryDeleteOutput, error) {
		out, err := tools.delete(ctx, args)
		if err != nil {
			return nil, memoryDeleteOutput{}, err
		}
		return nil, out, nil
	})
}
