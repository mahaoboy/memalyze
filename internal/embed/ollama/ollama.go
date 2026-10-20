// Package ollama 提供基于本地 Ollama 的 Embedder 实现。
//
// 只依赖 Ollama 的 HTTP API（/api/embed），不引入 SDK：
// 单二进制、无额外依赖，且离线可用。
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultModel 是默认的本地 embedding 模型（768 维）。
const DefaultModel = "nomic-embed-text"

// Client 是 Ollama 的 embedding 客户端。
type Client struct {
	baseURL     string
	model       string
	dim         int
	docPrefix   string // 写入侧前缀（nomic 需要 "search_document: "）
	queryPrefix string // 检索侧前缀（nomic 需要 "search_query: "）
	keepAlive   string // 传给 Ollama 的 keep_alive，避免空闲后重新加载模型
	http        *http.Client
}

// Option 用于配置 Client。
type Option func(*Client)

// WithDim 指定向量维度（默认 768，与 nomic-embed-text 一致）。
func WithDim(dim int) Option { return func(c *Client) { c.dim = dim } }

// WithHTTPClient 注入自定义 HTTP 客户端（测试或调优超时用）。
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithKeepAlive 设置 Ollama 的 keep_alive（如 "30m"、"2h"）。
//
// 默认 30m：embedding 模型冷加载约 20s，若空闲后立刻卸载，下一次查询会
// 被 3s 查询超时打成 degraded。传空字符串表示不发送该字段（用 Ollama 默认值）。
func WithKeepAlive(keepAlive string) Option {
	return func(c *Client) { c.keepAlive = keepAlive }
}

// WithPrefixes 设置写入侧 / 检索侧文本前缀。
//
// 这不是可选项而是正确性问题：nomic-embed-text 训练时用了
// "search_document: " 与 "search_query: " 前缀，不加会让检索质量明显下降
// （S8 实测：不加前缀 recall@3 只有 0.47）。
func WithPrefixes(doc, query string) Option {
	return func(c *Client) { c.docPrefix, c.queryPrefix = doc, query }
}

// New 创建客户端。baseURL 例：http://127.0.0.1:11434
func New(baseURL, model string, opts ...Option) *Client {
	if model == "" {
		model = DefaultModel
	}
	c := &Client{
		baseURL:   baseURL,
		model:     model,
		dim:       768,
		keepAlive: "30m",
		// 不在 http.Client 上写死超时：冷加载模型可能超过 10s，真正的
		// 截止时间由调用方的 context 决定（写入 30s / 检索 3s / warmup 90s）。
		// 这样 S14 才能既保留 3s 查询预算，又让冷启动 warmup 等到模型加载完。
		http: &http.Client{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type embedRequest struct {
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	KeepAlive string   `json:"keep_alive,omitempty"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed 批量生成向量。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: c.model, Input: texts, KeepAlive: c.keepAlive})
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}

	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama: got %d embeddings for %d inputs", len(out.Embeddings), len(texts))
	}
	if len(out.Embeddings[0]) > 0 {
		c.dim = len(out.Embeddings[0]) // 以模型实际输出为准
	}
	return out.Embeddings, nil
}

// EmbedDocuments 按写入侧前缀（若有）生成向量。
func (c *Client) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return c.Embed(ctx, applyPrefix(texts, c.docPrefix))
}

// EmbedQuery 按检索侧前缀（若有）生成单条查询向量。
func (c *Client) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.Embed(ctx, applyPrefix([]string{text}, c.queryPrefix))
	if err != nil {
		return nil, err
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("ollama: expected 1 embedding, got %d", len(vecs))
	}
	return vecs[0], nil
}

func applyPrefix(texts []string, prefix string) []string {
	if prefix == "" {
		return texts
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = prefix + t
	}
	return out
}

// Model 返回模型标识。
func (c *Client) Model() string { return c.model }

// Dim 返回维度（首次 Embed 前为默认值）。
func (c *Client) Dim() int { return c.dim }
