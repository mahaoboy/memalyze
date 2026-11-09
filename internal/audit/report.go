package audit

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
)

// ReportFormat 是静态治理报告格式；与原始审计 export 的格式解析刻意分开。
type ReportFormat string

const (
	ReportHTML ReportFormat = "html"
	ReportJSON ReportFormat = "json"
)

func ParseReportFormat(s string) (ReportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "html":
		return ReportHTML, nil
	case "json":
		return ReportJSON, nil
	}
	return "", fmt.Errorf("audit: unknown report format %q (want html or json)", s)
}

// ReportFilters 只保留筛选条件，不包含数据库路径等本地敏感信息。
type ReportFilters struct {
	From    string   `json:"from,omitempty"`
	To      string   `json:"to,omitempty"`
	Tenant  string   `json:"tenant,omitempty"`
	Actions []string `json:"actions"`
}

type ReportCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type ReportOutcomes struct {
	RedactionHits   int64 `json:"redaction_hits"`
	RedactedEvents  int64 `json:"redacted_events"`
	DuplicateEvents int64 `json:"duplicate_events"`
	DegradedEvents  int64 `json:"degraded_events"`
}

// Report 是 HTML 与 JSON 共用的聚合 DTO。这里刻意没有原始事件字段。
type Report struct {
	SchemaVersion int                     `json:"schema_version"`
	GeneratedAt   string                  `json:"generated_at"`
	Filters       ReportFilters           `json:"filters"`
	EventCount    int64                   `json:"event_count"`
	Actors        []ReportCount           `json:"actors"`
	Actions       []ReportCount           `json:"actions"`
	Scopes        []ReportCount           `json:"scopes"`
	Outcomes      ReportOutcomes          `json:"outcomes"`
	Verification  store.AuditVerification `json:"verification"`
}

func NewReport(entries []store.AuditEntry, filters ReportFilters, verification store.AuditVerification, generatedAt time.Time) Report {
	r := Report{
		SchemaVersion: 1,
		GeneratedAt:   generatedAt.UTC().Format(time.RFC3339),
		Filters:       filters,
		EventCount:    int64(len(entries)),
		Actors:        make([]ReportCount, 0),
		Actions:       make([]ReportCount, 0),
		Scopes:        make([]ReportCount, 0),
		Verification:  verification,
	}
	if r.Filters.Actions == nil {
		r.Filters.Actions = []string{}
	}
	sort.Strings(r.Filters.Actions)
	actors := make(map[string]int64)
	actions := make(map[string]int64)
	scopes := make(map[string]int64)
	for _, e := range entries {
		actors[e.Actor]++
		actions[string(e.Action)]++
		scopes[e.Scope]++
		r.Outcomes.RedactionHits += int64(e.Redacted)
		if e.Redacted > 0 {
			r.Outcomes.RedactedEvents++
		}
		if e.Duplicate {
			r.Outcomes.DuplicateEvents++
		}
		if e.Degraded {
			r.Outcomes.DegradedEvents++
		}
	}
	r.Actors = sortedCounts(actors)
	r.Actions = sortedCounts(actions)
	r.Scopes = sortedCounts(scopes)
	return r
}

func sortedCounts(groups map[string]int64) []ReportCount {
	out := make([]ReportCount, 0, len(groups))
	for name, count := range groups {
		out = append(out, ReportCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func WriteReport(w io.Writer, r Report, format ReportFormat) error {
	switch format {
	case ReportJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case ReportHTML, "":
		return reportTemplate.Execute(w, r)
	}
	return fmt.Errorf("audit: unknown report format %q", format)
}

var reportTemplate = template.Must(template.New("governance-report").Funcs(template.FuncMap{
	"orAll": func(s string) string {
		if s == "" {
			return "(all)"
		}
		return s
	},
	"emptyLabel": func(s string) string {
		if s == "" {
			return "(unset)"
		}
		return s
	},
	"join": func(values []string) string {
		if len(values) == 0 {
			return "(all)"
		}
		return strings.Join(values, ", ")
	},
	"status": func(ok bool) string {
		if ok {
			return "PASS"
		}
		return "FAIL"
	},
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>memalyze governance report</title>
<style>
:root{color-scheme:light dark;--bg:#f6f8fa;--card:#fff;--text:#17202a;--muted:#5c6670;--line:#d8dee4;--ok:#137333;--bad:#b3261e}@media(prefers-color-scheme:dark){:root{--bg:#11161b;--card:#1a222a;--text:#e7edf3;--muted:#a7b0ba;--line:#35404a;--ok:#67d58a;--bad:#ff8a80}}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:15px/1.55 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}main{max-width:1100px;margin:0 auto;padding:32px 20px 56px}h1{font-size:28px;margin:0 0 6px}h2{font-size:19px;margin:0 0 14px}.muted{color:var(--muted)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px;margin:20px 0}.card,section{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:18px}.metric{font-size:26px;font-weight:700}.status{{if .Verification.OK}}color:var(--ok){{else}}color:var(--bad){{end}}section{margin-top:16px}table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:8px 10px;border-bottom:1px solid var(--line);vertical-align:top}th{color:var(--muted);font-weight:600}td.num,th.num{text-align:right;font-variant-numeric:tabular-nums}.wrap{overflow-wrap:anywhere}footer{margin-top:20px;color:var(--muted);font-size:13px}.two{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px}code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace}</style>
</head>
<body><main>
<h1>memalyze governance report</h1>
<div class="muted">Generated {{.GeneratedAt}} · schema v{{.SchemaVersion}} · static local report</div>
<div class="grid">
<div class="card"><div class="muted">Filtered events</div><div class="metric">{{.EventCount}}</div></div>
<div class="card"><div class="muted">Redaction hits</div><div class="metric">{{.Outcomes.RedactionHits}}</div></div>
<div class="card"><div class="muted">Duplicate events</div><div class="metric">{{.Outcomes.DuplicateEvents}}</div></div>
<div class="card"><div class="muted">Degraded searches</div><div class="metric">{{.Outcomes.DegradedEvents}}</div></div>
</div>
<section><h2>Filters and outcomes</h2><table>
<tr><th>From</th><td class="wrap">{{orAll .Filters.From}}</td><th>To</th><td class="wrap">{{orAll .Filters.To}}</td></tr>
<tr><th>Tenant</th><td class="wrap">{{orAll .Filters.Tenant}}</td><th>Actions</th><td class="wrap">{{join .Filters.Actions}}</td></tr>
<tr><th>Redacted events</th><td>{{.Outcomes.RedactedEvents}}</td><th>Redaction hits</th><td>{{.Outcomes.RedactionHits}}</td></tr>
<tr><th>Duplicate events</th><td>{{.Outcomes.DuplicateEvents}}</td><th>Degraded events</th><td>{{.Outcomes.DegradedEvents}}</td></tr>
</table></section>
<section><h2>Full-chain verification</h2><table>
<tr><th>Status</th><td class="status"><strong>{{status .Verification.OK}}</strong></td><th>Algorithm</th><td>{{.Verification.Algorithm}}</td></tr>
<tr><th>Global events</th><td>{{.Verification.EventCount}}</td><th>Last sequence</th><td>{{.Verification.LastSeq}}</td></tr>
<tr><th>Head</th><td colspan="3" class="wrap"><code>{{.Verification.Head}}</code></td></tr>
<tr><th>Checkpoint</th><td>{{.Verification.CheckpointStatus}}</td><th>First invalid sequence</th><td>{{if .Verification.FirstInvalidSeq}}{{.Verification.FirstInvalidSeq}}{{else}}—{{end}}</td></tr>
{{if .Verification.Reason}}<tr><th>Reason</th><td colspan="3" class="wrap">{{.Verification.Reason}}</td></tr>{{end}}
</table><p class="muted">Verification covers the complete unfiltered chain. Without a previously saved checkpoint, PASS means internal consistency only; it does not detect deletion of a valid suffix.</p></section>
<div class="two">
<section><h2>Events by actor</h2><table><tr><th>Actor</th><th class="num">Events</th></tr>{{range .Actors}}<tr><td class="wrap">{{emptyLabel .Name}}</td><td class="num">{{.Count}}</td></tr>{{else}}<tr><td colspan="2">No events</td></tr>{{end}}</table></section>
<section><h2>Events by action</h2><table><tr><th>Action</th><th class="num">Events</th></tr>{{range .Actions}}<tr><td class="wrap">{{emptyLabel .Name}}</td><td class="num">{{.Count}}</td></tr>{{else}}<tr><td colspan="2">No events</td></tr>{{end}}</table></section>
</div>
<section><h2>Events by scope</h2><table><tr><th>Scope</th><th class="num">Events</th></tr>{{range .Scopes}}<tr><td class="wrap">{{emptyLabel .Name}}</td><td class="num">{{.Count}}</td></tr>{{else}}<tr><td colspan="2">No events</td></tr>{{end}}</table></section>
<footer>No memory contents, memory IDs, query text, query hashes, query lengths, or audit detail are included. Actor and scope are stored metadata; actor is configuration-derived, not an authenticated identity. Aggregates and verification share one SQLite read snapshot. Use a static backup for retained evidence.</footer>
</main></body></html>
`))
