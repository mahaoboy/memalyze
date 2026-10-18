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
