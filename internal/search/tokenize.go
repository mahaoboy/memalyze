// Package search 提供检索相关的基础能力。
//
// v0.1 的关键约束来自 S8 实测：
//   - SQLite FTS5 自带的分词器不切中文（unicode61 把整句当一个 token），
//     中文查询在默认分词下几乎无法命中；
//   - trigram 只能匹配 3 字符以上，二字符词（"审计""记忆"）永远匹配不到；
//   - 因此在 Go 侧做「CJK 二元组预切分」再交给 unicode61，是最稳的保底路径。
package search

import (
	"strings"
	"unicode"
)

// BigramTokens 把字符串切成检索用 token：
//   - 连续 CJK 片段 → 重叠二元组（"记忆层" → ["记忆","忆层"]；单字保留单字）
//   - 连续拉丁字母/数字 → 整体保留（"tenant_id" 在按分隔符切后为 "tenant","id"）
//   - 其余字符（标点、空白、下划线等）作为分隔
//
// 之所以不做"词典分词"：一人项目维护不起词典，而且二元组对召回更宽容，
// 排序交给 bm25 即可（详见 04-技术路线/检索验证-S8）。
func BigramTokens(s string) []string {
	var tokens []string
	var han []rune
	var word []rune

	flushHan := func() {
		if len(han) == 0 {
			return
		}
		if len(han) == 1 {
			tokens = append(tokens, string(han))
		} else {
			for i := 0; i+1 < len(han); i++ {
				tokens = append(tokens, string(han[i:i+2]))
			}
		}
		han = han[:0]
	}
	flushWord := func() {
		if len(word) > 0 {
			tokens = append(tokens, string(word))
			word = word[:0]
		}
	}

	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.Is(unicode.Han, r):
			flushWord()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			word = append(word, r)
		default:
			flushHan()
			flushWord()
		}
	}
	flushHan()
	flushWord()
	return tokens
}

// BigramText 返回写入 FTS5 的文本：token 以空格连接，交给 unicode61 分词器建索引。
func BigramText(s string) string {
	return strings.Join(BigramTokens(s), " ")
}

// MatchExpr 把用户查询转成 FTS5 MATCH 表达式（token 级 OR + 短语引号转义）。
//
// 用 OR 而不是 AND：中文短查询切出多个二元组时，AND 过严会漏召回；
// 相关性由 bm25 排序处理。查询为空返回空串（调用方应跳过检索）。
func MatchExpr(q string) string {
	tokens := BigramTokens(q)
	if len(tokens) == 0 {
		return ""
	}
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
		quoted[i] = Quote(t)
	}
	return strings.Join(quoted, " OR ")
}

// Quote 把单个 token 转成 FTS5 短语，内部双引号做转义。
func Quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
