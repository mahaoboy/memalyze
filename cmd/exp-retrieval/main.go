// Command exp-retrieval 是 S8 的检索方案对比实验（一次性验证工具，不是产品代码）。
//
// 在 60 条真实感中文"记忆"上比较：
//  1. 纯向量（可多模型/多前缀组合）
//  2. FTS5 默认分词（unicode61）
//  3. FTS5 trigram
//  4. FTS5 二元组（CJK bigram 预切分 + unicode61）
//  5. RRF 混合（向量 + 最佳 FTS）
//
// 运行（需本机 Ollama）：
//
//	go run ./cmd/exp-retrieval
package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/embed"
	"github.com/mahaoboy/memalyze/internal/embed/ollama"
	"github.com/mahaoboy/memalyze/internal/search"

	_ "modernc.org/sqlite"
)

const (
	nomicModel = "nomic-embed-text"
	qwenModel  = "qwen3-embedding:0.6b"
)

// corpus 是 60 条贴近真实场景的中文"记忆"。
var corpus = []string{
	"主力语言定为 Go，为了单二进制分发",
	"存储默认用 SQLite，单文件零依赖",
	"向量先存 BLOB，用暴力余弦检索",
	"tenant_id 从第一天就要预留，避免商用版迁移",
	"记忆支持 TTL 和软删除，满足被遗忘权",
	"许可证定为 AGPL-3.0 加商业双许可",
	"部署在新加坡的一台服务器上，作为境外入口",
	"每天投入两小时，周六做复盘和排期",
	"周日强制休息，不写代码",
	"暴露给 harness 的工具控制在五个以内",
	"embedding 用本地 nomic-embed-text，768 维",
	"mem0 的隔离只到 user_id，没有租户概念",
	"Basic Memory 装了 173 个 Python 包",
	"Zep 的社区版已经不再支持自托管",
	"开源版负责获客，商用版负责收入",
	"不做 SaaS 托管，只做自托管加 License Key",
	"证据强度要标清楚，弱证据不能当结论",
	"先发布再迭代，别憋大招",
	"第十周完成 Docker 化，五分钟跑起来",
	"发布渠道考虑 GitHub、Hacker News、Reddit、V2EX",
	"用户偏好直接给结论，不要长篇解释",
	"写文档要写清前因后果，方便半年后回看",
	"反向代理用 Caddy，自动申请证书",
	"备份策略是每天一次全量，保留七天",
	"审计日志要能导出，按时间倒序",
	"多租户隔离在 Store 层强制，不暴露裸 SQL",
	"检索先用向量，再融合 FTS5 关键词",
	"RRF 的 k 取 60，先不做权重调参",
	"FTS5 的默认分词器对中文不友好，考虑 trigram",
	"超过十万条再考虑 HNSW 或 pgvector",
	"单条记忆的向量是 768 维 float32，约 3KB",
	"一条记忆可以有多个模型的多套向量",
	"去重用 content_hash 精确匹配",
	"scope 用冒号分层，比如 user:alice",
	"检索必须带 tenant_id 和 scope 条件",
	"关键词检索对精确词更可靠，向量对改写更鲁棒",
	"冷启动问题：新库没有向量，只能走关键词",
	"用户反馈记了但取不到，说明检索质量有问题",
	"灰度发布先在测试实例上跑三天",
	"定价先定每年一千九百九十九",
	"发票和合同模板放到第三阶段再做",
	"法律红线：不做数据出境，不碰跨境信道",
	"一人公司最大的敌人是做了没人要的东西",
	"每周三固定两小时处理 issue 和社区",
	"issue 积压超过二十条就暂停新功能",
	"GitHub Actions 里加 CGO_ENABLED=0 构建检查",
	"交叉编译出 linux amd64 和 arm64 两个包",
	"goreleaser 负责打 tag 和生成 changelog",
	"配置文件用环境变量加 yaml，容器友好",
	"日志用标准库 log/slog，结构化输出",
	"可观测性放到 M2 之后，先不接 OpenTelemetry",
	"用户在上海，工作日晚上写代码",
	"第二阶段的指标是十到五十个活跃实例",
	"北极星指标是周活跃自部署实例数",
	"反馈收集至少十条才算验证",
	"阶段复盘要回答继续还是转向",
	"如果竞争对手推出企业自托管版就重新评估",
	"中文二字符词是分词难点，比如记忆、检索",
	"离线可用是卖点，不依赖外网下载模型",
	"目标年收入二十万到两百万",
}

type query struct {
	text string
	gold []int
}

var queries = []query{
	{"记忆层用什么语言写的", []int{0}},
	{"多租户字段什么时候加", []int{3}},
	{"什么时候才需要向量索引", []int{29}},
	{"开源许可证选了哪个", []int{5}},
	{"哪天强制休息", []int{8}},
	{"mem0 的隔离粒度", []int{11}},
	{"中文分词有什么坑", []int{28, 57}},
	{"服务器放在哪里", []int{6}},
	{"工具数量上限是多少", []int{9}},
	{"复盘安排在什么时候", []int{7}},
	{"embedding 模型是什么", []int{10}},
	{"docker 化的验收标准", []int{18}},
	{"tenant_id", []int{3}},
	{"AGPL", []int{5}},
	{"RRF", []int{27}},
	{"审计", []int{24}},
}

type hit struct {
	id    int
	score float64
}

// source 是一路"向量召回"来源（模型 + 前缀组合）。
type source struct {
	name  string
	vecs  [][]float32
	qvecs [][]float32
	dim   int
}

func main() {
	baseURL := os.Getenv("OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	qs := make([]string, len(queries))
	for i, q := range queries {
		qs[i] = q.text
	}

	var sources []source
	// nomic 不加前缀（错误用法）与加前缀（正确用法）对比
	sources = append(sources, buildSource(baseURL, "nomic/raw", nomicModel, "", "", corpus, qs))
	sources = append(sources,
		buildSource(baseURL, "nomic/prefix", nomicModel, "search_document: ", "search_query: ", corpus, qs))
	if modelAvailable(baseURL, qwenModel) {
		sources = append(sources, buildSource(baseURL, "qwen3-0.6b", qwenModel, "", "", corpus, qs))
		// Qwen3-Embedding 官方建议查询侧加 "query: " 指令前缀
		sources = append(sources,
			buildSource(baseURL, "qwen3/prefix", qwenModel, "", "query: ", corpus, qs))
	}

	fmt.Printf("语料 %d 条，查询 %d 个\n\n", len(corpus), len(queries))

	names, results := runAll(sources)
	report(names, results)
}

// buildSource 建库：语料与查询都预先向量化（含前缀）。
func buildSource(baseURL, name, model, docPrefix, queryPrefix string, docs, qs []string) source {
	client := ollama.New(baseURL, model,
		ollama.WithHTTPClient(&http.Client{Timeout: 300 * time.Second}),
		ollama.WithPrefixes(docPrefix, queryPrefix),
	)

	start := time.Now()
	// 预热：首次调用要把模型读进内存（本机实测 >15s）
	if _, err := client.Embed(context.Background(), []string{"warmup"}); err != nil {
		fmt.Fprintf(os.Stderr, "%s 预热失败: %v\n", name, err)
		os.Exit(1)
	}
	vecs, err := client.EmbedDocuments(context.Background(), docs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s 文档向量失败: %v\n", name, err)
		os.Exit(1)
	}
	qvecs, err := client.Embed(context.Background(), prefixed(qs, queryPrefix))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s 查询向量失败: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] 建库 %d 条 / %d 维 / %dms（含预热）\n", name, len(vecs), len(vecs[0]), time.Since(start).Milliseconds())
	return source{name: name, vecs: vecs, qvecs: qvecs, dim: len(vecs[0])}
}

func prefixed(texts []string, prefix string) []string {
	if prefix == "" {
		return texts
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = prefix + t
	}
	return out
}

func modelAvailable(baseURL, model string) bool {
	resp, err := http.Get(baseURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return strings.Contains(string(b), strings.Split(model, ":")[0])
}

// runAll 生成所有 variant 的检索结果。
func runAll(sources []source) ([]string, [][][]hit) {
	var names []string
	var out [][][]hit

	db := mustDB()
	defer db.Close()
	newFTS(db, "fts_uni", "", corpus)
	newFTS(db, "fts_tri", "tokenize='trigram'", corpus)
	biDocs := make([]string, len(corpus))
	for i, d := range corpus {
		biDocs[i] = search.BigramText(d)
	}
	newFTS(db, "fts_bi", "", biDocs)

	ftsHits := func(table, mode string) func(int) []hit {
		return func(qi int) []hit {
			q := queries[qi].text
			switch mode {
			case "bigram":
				return queryFTS(db, table, search.MatchExpr(q))
			default:
				return queryFTS(db, table, quote(q))
			}
		}
	}

	add := func(name string, fn func(int) []hit) {
		per := make([][]hit, len(queries))
		for i := range queries {
			per[i] = fn(i)
		}
		names = append(names, name)
		out = append(out, per)
	}

	bigram := ftsHits("fts_bi", "bigram")

	for _, s := range sources {
		s := s
		vector := func(qi int) []hit {
			idx, scores := embed.TopK(s.qvecs[qi], s.vecs, 10)
			hits := make([]hit, len(idx))
			for i := range idx {
				hits[i] = hit{id: idx[i], score: scores[i]}
			}
			return hits
		}
		add("vector:"+s.name, vector)
		add("rrf("+s.name+",bigram)", rrf(vector, bigram, 60, 10))
	}

	add("fts-unicode61", ftsHits("fts_uni", "unicode61"))
	add("fts-trigram", ftsHits("fts_tri", "trigram"))
	add("fts-bigram", bigram)
	return names, out
}

// rrf 用倒数排名融合两路结果（k=60，先不做权重调参）。
func rrf(a, b func(int) []hit, k, limit int) func(int) []hit {
	return func(qi int) []hit {
		score := map[int]float64{}
		for _, hs := range [][]hit{a(qi), b(qi)} {
			for rank, h := range hs {
				score[h.id] += 1.0 / float64(k+rank+1)
			}
		}
		ids := make([]int, 0, len(score))
		for id := range score {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return score[ids[i]] > score[ids[j]] })
		if len(ids) > limit {
			ids = ids[:limit]
		}
		hits := make([]hit, len(ids))
		for i, id := range ids {
			hits[i] = hit{id: id, score: score[id]}
		}
		return hits
	}
}

// ---- 指标 ----

func recallAt(hits []hit, gold []int, k int) float64 {
	if len(gold) == 0 {
		return 0
	}
	found := 0
	for _, g := range gold {
		for i, h := range hits {
			if i >= k {
				break
			}
			if h.id == g {
				found++
				break
			}
		}
	}
	return float64(found) / float64(len(gold))
}

func mrr(hits []hit, gold []int) float64 {
	for i, h := range hits {
		for _, g := range gold {
			if h.id == g {
				return 1.0 / float64(i+1)
			}
		}
	}
	return 0
}

func report(names []string, results [][][]hit) {
	fmt.Printf("%-30s %9s %8s\n", "variant", "recall@3", "mrr@10")
	fmt.Println(strings.Repeat("-", 50))
	for vi, name := range names {
		var r3, m float64
		for qi, q := range queries {
			r3 += recallAt(results[vi][qi], q.gold, 3)
			m += mrr(results[vi][qi], q.gold)
		}
		fmt.Printf("%-30s %9.2f %8.2f\n", name, r3/float64(len(queries)), m/float64(len(queries)))
	}

	fmt.Println("\n=== 失败样例（每 variant 最多 2 条）===")
	for vi, name := range names {
		shown := 0
		for qi, q := range queries {
			if recallAt(results[vi][qi], q.gold, 3) > 0 {
				continue
			}
			fmt.Printf("[%s] %q 期望 %v 实际 %v\n", name, q.text, q.gold, idsOf(results[vi][qi], 3))
			shown++
			if shown == 2 {
				break
			}
		}
	}
}

func idsOf(hits []hit, n int) []int {
	if len(hits) < n {
		n = len(hits)
	}
	ids := make([]int, n)
	for i := 0; i < n; i++ {
		ids[i] = hits[i].id
	}
	return ids
}

// ---- SQLite / FTS5 ----

func mustDB() *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1) // :memory: 必须用同一连接
	if _, err := db.Exec(`CREATE TABLE memories(id INTEGER PRIMARY KEY, content TEXT NOT NULL)`); err != nil {
		panic(err)
	}
	return db
}

func newFTS(db *sql.DB, table, tokenize string, docs []string) {
	ddl := fmt.Sprintf(`CREATE VIRTUAL TABLE %s USING fts5(content)`, table)
	if tokenize != "" {
		ddl = fmt.Sprintf(`CREATE VIRTUAL TABLE %s USING fts5(content, %s)`, table, tokenize)
	}
	if _, err := db.Exec(ddl); err != nil {
		panic(fmt.Sprintf("create %s: %v", table, err))
	}
	for i, d := range docs {
		if _, err := db.Exec(fmt.Sprintf(`INSERT INTO %s(rowid, content) VALUES (?, ?)`, table), i, d); err != nil {
			panic(fmt.Sprintf("insert %s: %v", table, err))
		}
	}
}

func queryFTS(db *sql.DB, table, expr string) []hit {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	q := fmt.Sprintf(`SELECT rowid, bm25(%s) AS rank FROM %s WHERE %s MATCH ? ORDER BY rank LIMIT 10`, table, table, table)
	rows, err := db.Query(q, expr)
	if err != nil {
		return nil // 语法错误 / 无匹配都记为空结果
	}
	defer rows.Close()
	var hits []hit
	for rows.Next() {
		var id int
		var rank float64
		if err := rows.Scan(&id, &rank); err != nil {
			continue
		}
		hits = append(hits, hit{id: id, score: -rank})
	}
	return hits
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
