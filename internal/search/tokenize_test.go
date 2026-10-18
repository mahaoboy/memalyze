package search

import (
	"reflect"
	"testing"
)

func TestBigramTokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"空串", "", nil},
		{"纯中文偶数", "记忆层", []string{"记忆", "忆层"}},
		{"纯中文单字", "记", []string{"记"}},
		{"纯中文单字加标点", "记。", []string{"记"}},
		{"中英混排", "用 Go 写", []string{"用", "go", "写"}},
		{"下划线分词", "tenant_id 要预留", []string{"tenant", "id", "要预", "预留"}},
		{"大小写归一", "AGPL-3.0", []string{"agpl", "3", "0"}},
		{"标点作分隔", "多租户、审计：TTL", []string{"多租", "租户", "审计", "ttl"}},
	}
	for _, c := range cases {
		if got := BigramTokens(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: BigramTokens(%q) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestBigramText(t *testing.T) {
	if got, want := BigramText("记忆层"), "记忆 忆层"; got != want {
		t.Fatalf("BigramText = %q, want %q", got, want)
	}
}

func TestMatchExpr(t *testing.T) {
	got := MatchExpr("记忆层")
	want := `"记忆" OR "忆层"`
	if got != want {
		t.Fatalf("MatchExpr = %q, want %q", got, want)
	}
	if MatchExpr("") != "" {
		t.Fatal("empty query should produce empty expr")
	}
}

func TestMatchExprTreatsQuotesAsSeparators(t *testing.T) {
	// 双引号属于分隔符，不会被切进 token；Quote 的转义是防御性的。
	// 这条测试锁住"用户输入里的引号不会破坏 MATCH 表达式"这一行为。
	got := MatchExpr(`he"llo`)
	want := `"he" OR "llo"`
	if got != want {
		t.Fatalf("MatchExpr = %q, want %q", got, want)
	}
}
