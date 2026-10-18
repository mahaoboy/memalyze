// Package embed 提供 embedding 的编码与相似度计算。
//
// 设计约束（见决策 D-008）：
//   - 向量以 float32 小端连续数组存进 SQLite BLOB，不依赖任何向量扩展
//   - 相似度先用暴力余弦，< 1 万条毫秒级
package embed

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Dim 是 float32 的字节宽度。
const Dim = 4

// Encode 把向量编码为小端 float32 字节串，可直接写入 SQLite BLOB。
//
// 编码是定长的：len(b) == 4*len(vec)，解码时靠这个反推维度。
func Encode(vec []float32) []byte {
	buf := make([]byte, Dim*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*Dim:], math.Float32bits(v))
	}
	return buf
}

// Decode 把 BLOB 还原为向量；字节长度不是 4 的整数倍时报错。
func Decode(b []byte) ([]float32, error) {
	if len(b)%Dim != 0 {
		return nil, fmt.Errorf("embed: blob length %d is not a multiple of %d", len(b), Dim)
	}
	vec := make([]float32, len(b)/Dim)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*Dim:]))
	}
	return vec, nil
}

// Cosine 计算两个向量的余弦相似度。
//
// 维度不一致返回错误而不是 panic —— 换 embedding 模型时这是最常见的线上事故。
// 任一向量为零向量时返回 0（不做 NaN 传染）。
func Cosine(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("embed: dimension mismatch: %d vs %d", len(a), len(b))
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), nil
}

// TopK 返回与 query 最相似的 k 个下标及其分数，按分数降序。
//
// 调用方负责保证 len(vectors) 与业务数据对齐（下标即行序）。
func TopK(query []float32, vectors [][]float32, k int) ([]int, []float64) {
	type scored struct {
		idx int
		sim float64
	}
	hits := make([]scored, 0, len(vectors))
	for i, v := range vectors {
		sim, err := Cosine(query, v)
		if err != nil {
			continue // 维度不一致的行直接跳过，宁可少召回也不 panic
		}
		hits = append(hits, scored{idx: i, sim: sim})
	}
	// 数据量小（< 1 万），用简单选择而非全排序换可读性
	sortDesc(hits, func(h scored) float64 { return h.sim })
	if k > len(hits) {
		k = len(hits)
	}
	out := hits[:k]
	idx := make([]int, len(out))
	scores := make([]float64, len(out))
	for i, h := range out {
		idx[i], scores[i] = h.idx, h.sim
	}
	return idx, scores
}

func sortDesc[T any](s []T, score func(T) float64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && score(s[j]) > score(s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
