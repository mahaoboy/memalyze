// Package govern 实现「写入前脱敏」的规则引擎（S15）。
//
// 设计约束（见 07-执行计划/S15-企业治理目标 与 D-020）：
//   - 脱敏必须发生在**去重与 embedding 之前**，否则原文会经 FTS / 向量泄漏
//   - v0.1 默认**不可逆**：不保存原文、不保存密钥
//   - 规则可配置（JSON），内置一套保守默认；宁可少脱敏，不可误杀正常内容
//
// 规则匹配全部用 Go 标准库 regexp（RE2），因此不支持反向断言；
// 需要边界时用 \b 或把前缀并进替换模板（$1）。
package govern

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Mode 是脱敏模式。
type Mode string

const (
	// ModeOff 关闭脱敏（仅用于排障；生产不建议）。
	ModeOff Mode = "off"
	// ModeDryRun 只统计"本应脱敏"的次数，不改写内容，用于观察误杀。
	ModeDryRun Mode = "dry-run"
	// ModeEnforce 真正改写内容（默认）。
	ModeEnforce Mode = "enforce"
)

// Valid 报告模式是否合法。
func (m Mode) Valid() bool {
	switch m {
	case ModeOff, ModeDryRun, ModeEnforce:
		return true
	}
	return false
}

// Rule 是一条脱敏规则。
type Rule struct {
	// Name 是规则名，会写进结果与审计（不含原文，安全）。
	Name string `json:"name"`
	// Pattern 是 RE2 正则。
	Pattern string `json:"pattern"`
	// Replacement 支持 $1 之类的捕获组引用。
	Replacement string `json:"replacement"`
	// Enabled 缺省为 true；显式 false 用来临时关闭单条规则。
	Enabled *bool `json:"enabled,omitempty"`
}

// EnabledOr 报告规则是否启用。
func (r Rule) EnabledOr() bool { return r.Enabled == nil || *r.Enabled }

// Hit 是一条规则的命中统计。
type Hit struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Redactor 是编译后的规则集合，可并发使用（regexp 本身安全）。
type Redactor struct {
	rules []compiled
}

type compiled struct {
	name string
	re   *regexp.Regexp
	repl string
}

// DefaultRules 返回内置的保守规则集。
//
// 顺序有意义：长的、结构强的规则排在前面，避免被短规则先切碎
// （例如 18 位身份证必须先于 11 位手机号匹配）。
func DefaultRules() []Rule {
	return []Rule{
		{
			Name:        "private_key_block",
			Pattern:     `-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`,
			Replacement: "[REDACTED:private_key]",
		},
		{
			Name:        "id_card_cn",
			Pattern:     `\b[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`,
			Replacement: "[REDACTED:id_card]",
		},
		{
			Name:        "bank_card",
			Pattern:     `\b(?:62\d{14,17}|4\d{15}|5[1-5]\d{14})\b`,
			Replacement: "[REDACTED:bank_card]",
		},
		{
			Name:        "jwt",
			Pattern:     `\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`,
			Replacement: "[REDACTED:jwt]",
		},
		{
			Name:        "api_key_known_prefix",
			Pattern:     `\b(?:sk-[A-Za-z0-9_-]{16,}|ghp_[A-Za-z0-9]{20,}|gho_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{10,})\b`,
			Replacement: "[REDACTED:api_key]",
		},
		{
			Name: "credential_assignment",
			// 保留键名，只抹掉值：api_key=xxx → api_key=[REDACTED:credential]
			Pattern:     `(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|secret[_-]?key|password|passwd)\b\s*[:=]\s*["']?[^\s"',;]{6,}["']?`,
			Replacement: `$1=[REDACTED:credential]`,
		},
		{
			Name:        "email",
			Pattern:     `[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`,
			Replacement: "[REDACTED:email]",
		},
		{
			Name:        "phone_cn",
			Pattern:     `(?:\+?86[-\s]?)?\b1[3-9]\d{9}\b`,
			Replacement: "[REDACTED:phone]",
		},
	}
}

// New 编译规则集。空规则集返回错误：静默放行比脱敏失败更危险。
func New(rules []Rule) (*Redactor, error) {
	if len(rules) == 0 {
		return nil, errors.New("govern: empty rule set")
	}
	r := &Redactor{rules: make([]compiled, 0, len(rules))}
	seen := make(map[string]bool, len(rules))
	for i, rule := range rules {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return nil, fmt.Errorf("govern: rule #%d: empty name", i+1)
		}
		if seen[name] {
			return nil, fmt.Errorf("govern: duplicate rule name %q", name)
		}
		seen[name] = true
		if !rule.EnabledOr() {
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("govern: rule %q: %w", name, err)
		}
		repl := rule.Replacement
		if repl == "" {
			repl = "[REDACTED:" + name + "]"
		}
		r.rules = append(r.rules, compiled{name: name, re: re, repl: repl})
	}
	if len(r.rules) == 0 {
		return nil, errors.New("govern: all rules disabled")
	}
	return r, nil
}

// Default 用内置规则构造 Redactor。
func Default() (*Redactor, error) { return New(DefaultRules()) }

// Names 返回启用的规则名（按应用顺序）。
func (r *Redactor) Names() []string {
	out := make([]string, 0, len(r.rules))
	for _, c := range r.rules {
		out = append(out, c.name)
	}
	return out
}

// Redact 依次应用规则，返回脱敏后的文本与命中统计（按规则名聚合，只含有命中的）。
func (r *Redactor) Redact(s string) (string, []Hit) {
	if r == nil || s == "" {
		return s, nil
	}
	counts := make(map[string]int, len(r.rules))
	out := s
	for _, c := range r.rules {
		if n := len(c.re.FindAllStringIndex(out, -1)); n > 0 {
			counts[c.name] += n
			out = c.re.ReplaceAllString(out, c.repl)
		}
	}
	if len(counts) == 0 {
		return out, nil
	}
	hits := make([]Hit, 0, len(counts))
	for name, n := range counts {
		hits = append(hits, Hit{Name: name, Count: n})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Name < hits[j].Name })
	return out, hits
}

// Total 汇总命中次数。
func Total(hits []Hit) int {
	n := 0
	for _, h := range hits {
		n += h.Count
	}
	return n
}

// Names 返回命中规则名（去重、有序）。
func HitNames(hits []Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Name)
	}
	return out
}

// ParseRules 解析 JSON 规则文件。
//
// 支持两种形状：{"rules":[...]} 或顶层数组 [...]。
func ParseRules(data []byte) ([]Rule, error) {
	var wrapper struct {
		Rules []Rule `json:"rules"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Rules != nil {
		return wrapper.Rules, nil
	}
	var list []Rule
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("govern: parse rules: %w", err)
	}
	return list, nil
}

// LoadRulesFile 从文件读规则；path 为空时用内置规则。
func LoadRulesFile(path string) ([]Rule, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultRules(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("govern: read rules %s: %w", path, err)
	}
	rules, err := ParseRules(data)
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// Load 按文件路径构造 Redactor（空路径 = 内置规则）。
func Load(path string) (*Redactor, error) {
	rules, err := LoadRulesFile(path)
	if err != nil {
		return nil, err
	}
	return New(rules)
}
