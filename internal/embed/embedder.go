// Package embed 定义文本向量化接口。
//
// v0.1 只实现 Ollama（本地模型，离线可用、无需外网）——
// 这是与竞品的差异点之一：不依赖 HuggingFace 下载。
package embed

import "context"

// Embedder 把一批文本转成同维度向量。
//
// 实现约定：
//   - 返回的切片长度必须与输入等长且顺序一致
//   - 任何一条失败即整体返回错误（不做部分成功），避免下标错位
type Embedder interface {
	// Embed 返回每段文本的向量。
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Model 返回模型标识，落库时写进 embeddings.model，换模型可区分。
	Model() string
	// Dim 返回向量维度；未知时返回 0。
	Dim() int
}

// DocumentEmbedder 是可选能力：写入侧向量化（可能带 "search_document: " 之类前缀）。
//
// 调用方应先断言是否实现该接口，再退化为 Embed —— 这样 Embedder 接口保持最小，
// 而需要前缀的模型（nomic）也能正确工作。
type DocumentEmbedder interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
}

// QueryEmbedder 是可选能力：检索侧向量化（可能带 "search_query: " / "query: " 前缀）。
//
// 单独成接口的原因：S8 实测显示前缀对部分模型是正确性问题（nomic），
// 对另一部分是可选优化（qwen）——两条路径必须能分别配置。
type QueryEmbedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// EmbedDocuments 优先走 DocumentEmbedder，否则退化为 Embed。
func EmbedDocuments(ctx context.Context, e Embedder, texts []string) ([][]float32, error) {
	if de, ok := e.(DocumentEmbedder); ok {
		return de.EmbedDocuments(ctx, texts)
	}
	return e.Embed(ctx, texts)
}

// EmbedQuery 优先走 QueryEmbedder，否则退化为单条 Embed。
func EmbedQuery(ctx context.Context, e Embedder, text string) ([]float32, error) {
	if qe, ok := e.(QueryEmbedder); ok {
		return qe.EmbedQuery(ctx, text)
	}
	vecs, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) != 1 {
		return nil, errUnexpectedCount(len(vecs))
	}
	return vecs[0], nil
}

func errUnexpectedCount(n int) error {
	return &countError{n: n}
}

type countError struct{ n int }

func (e *countError) Error() string {
	return "embed: expected exactly 1 embedding, got " + itoa(e.n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
