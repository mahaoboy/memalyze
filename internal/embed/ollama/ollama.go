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
	"time"
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
	http        *http.Client
}

// Option 用于配置 Client。
type Option func(*Client)

// WithDim 指定向量维度（默认 768，与 nomic-embed-text 一致）。
func WithDim(dim int) Option { return func(c *Client) { c.dim = dim } }

// WithHTTPClient 注入自定义 HTTP 客户端（测试或调优超时用）。
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

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
		baseURL: baseURL,
		model:   model,
		dim:     768,
		// 超时是硬要求（S7 教训：竞品语义检索能卡 60s）。
		// 单批 embedding 超 10s 视为不可用，调用方据此降级到关键词检索。
		http: &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed 批量生成向量。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: c.model, Input: texts})
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
