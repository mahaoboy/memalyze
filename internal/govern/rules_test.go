package govern

import (
	"strings"
	"testing"
)

func mustDefault(t *testing.T) *Redactor {
	t.Helper()
	r, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	return r
}

func TestDefaultRulesHitSensitiveData(t *testing.T) {
	r := mustDefault(t)

	cases := []struct {
		name string
		in   string
		rule string
		want string
	}{
		{
			name: "email",
			in:   "联系我 zhangsan.li+work@example.co.uk 谢谢",
			rule: "email",
			want: "联系我 [REDACTED:email] 谢谢",
		},
		{
			name: "手机号",
			in:   "我的手机 13800138000，随时打",
			rule: "phone_cn",
			want: "我的手机 [REDACTED:phone]，随时打",
		},
		{
			name: "手机号带国际区号",
			in:   "call +86 13912345678 now",
			rule: "phone_cn",
			want: "call [REDACTED:phone] now",
		},
		{
			name: "身份证",
			in:   "身份证号 110101199003071234 已核验",
			rule: "id_card_cn",
			want: "身份证号 [REDACTED:id_card] 已核验",
		},
		{
			name: "银行卡",
			in:   "卡号 6222021234567890123 转账",
			rule: "bank_card",
			want: "卡号 [REDACTED:bank_card] 转账",
		},
		{
			name: "已知前缀 API key",
			in:   "export OPENAI=sk-abcdefghijklmnopqrstuvwxyz123456",
			rule: "api_key_known_prefix",
			want: "export OPENAI=[REDACTED:api_key]",
		},
		{
			name: "JWT",
			in:   "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			rule: "jwt",
			want: "Authorization: Bearer [REDACTED:jwt]",
		},
		{
			name: "密码赋值保留键名",
			in:   "配置: api_key = 'abcdef1234567890' 完了",
			rule: "credential_assignment",
			want: "配置: api_key=[REDACTED:credential] 完了",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, hits := r.Redact(tc.in)
			if got != tc.want {
				t.Fatalf("Redact()\n got: %q\nwant: %q", got, tc.want)
			}
			if len(hits) != 1 || hits[0].Name != tc.rule || hits[0].Count != 1 {
				t.Fatalf("hits = %+v, want single %q", hits, tc.rule)
			}
		})
	}
}

func TestPrivateKeyBlock(t *testing.T) {
	r := mustDefault(t)
	in := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"
	got, hits := r.Redact(in)
	if got != "[REDACTED:private_key]" {
		t.Fatalf("got %q", got)
	}
	if Total(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestNoFalsePositiveOnNormalText(t *testing.T) {
	r := mustDefault(t)
	cases := []string{
		"每周日强制休息，每天 2 小时",
		"2026-10-25 01:15 提交了 commit 544957c",
		"FTS5 二元组召回 0.94，qwen3-embedding:0.6b 是 1024 维",
		"订单号 202610250001 已发货",
		"端口 127.0.0.1:11434 监听中",
		"版本 v1.8.0，覆盖率 84.3%，预算 ¥1 万",
		"ULID 01J8Z3K4M5N6P7Q8R9S0T1V2W3 可排序",
		"memory_add / memory_search / memory_delete 三个工具",
	}
	for _, in := range cases {
		got, hits := r.Redact(in)
		if got != in || len(hits) != 0 {
			t.Errorf("误杀: %q → %q (hits=%+v)", in, got, hits)
		}
	}
}

func TestRedactIsIdempotent(t *testing.T) {
	r := mustDefault(t)
	in := "a@b.com / 13800138000 / 110101199003071234"
	once, _ := r.Redact(in)
	twice, hits := r.Redact(once)
	if twice != once {
		t.Fatalf("not idempotent: %q → %q", once, twice)
	}
	if len(hits) != 0 {
		t.Fatalf("second pass should not hit, got %+v", hits)
	}
}

func TestCountsAggregate(t *testing.T) {
	r := mustDefault(t)
	_, hits := r.Redact("a@b.com c@d.com 13800138000")
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
	byName := map[string]int{}
	for _, h := range hits {
		byName[h.Name] = h.Count
	}
	if byName["email"] != 2 || byName["phone_cn"] != 1 {
		t.Fatalf("unexpected counts: %+v", byName)
	}
}

func TestParseRulesShapes(t *testing.T) {
	obj := []byte(`{"rules":[{"name":"a","pattern":"x+","replacement":"[A]"}]}`)
	rules, err := ParseRules(obj)
	if err != nil || len(rules) != 1 || rules[0].Name != "a" {
		t.Fatalf("object form: %+v, %v", rules, err)
	}
	arr := []byte(`[{"name":"b","pattern":"y+","replacement":"[B]"}]`)
	rules, err = ParseRules(arr)
	if err != nil || len(rules) != 1 || rules[0].Name != "b" {
		t.Fatalf("array form: %+v, %v", rules, err)
	}
	if _, err := ParseRules([]byte(`{`)); err == nil {
		t.Fatal("expected error for malformed json")
	}
}

func TestRuleValidation(t *testing.T) {
	disabled := false
	cases := []struct {
		name  string
		rules []Rule
		want  string
	}{
		{"empty", nil, "empty rule set"},
		{"empty name", []Rule{{Pattern: "a"}}, "empty name"},
		{"duplicate", []Rule{{Name: "a", Pattern: "a"}, {Name: "a", Pattern: "b"}}, "duplicate"},
		{"bad regexp", []Rule{{Name: "a", Pattern: "("}}, "error parsing regexp"},
		{"all disabled", []Rule{{Name: "a", Pattern: "a", Enabled: &disabled}}, "all rules disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.rules)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestDisabledRuleSkipped(t *testing.T) {
	disabled := false
	r, err := New([]Rule{
		{Name: "off", Pattern: "secret", Replacement: "[X]", Enabled: &disabled},
		{Name: "on", Pattern: "token", Replacement: "[T]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, hits := r.Redact("secret token")
	if got != "secret [T]" {
		t.Fatalf("got %q", got)
	}
	if len(hits) != 1 || hits[0].Name != "on" {
		t.Fatalf("hits = %+v", hits)
	}
	if names := r.Names(); len(names) != 1 || names[0] != "on" {
		t.Fatalf("names = %v", names)
	}
}
