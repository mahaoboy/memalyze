package embed

import (
	"context"
	"errors"
	"testing"
)

type plainEmbedder struct {
	vecs    [][]float32
	err     error
	n       int
	lastLen int
}

func (p *plainEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	p.n++
	p.lastLen = len(texts)
	if p.err != nil {
		return nil, p.err
	}
	if p.vecs != nil {
		return p.vecs, nil
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 2}
	}
	return out, nil
}
func (p *plainEmbedder) Model() string { return "plain" }
func (p *plainEmbedder) Dim() int      { return 2 }

type docEmbedder struct {
	plainEmbedder
	docCalls int
}

func (d *docEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	d.docCalls++
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{9, 9}
	}
	return out, nil
}

type queryEmbedder struct {
	plainEmbedder
	queryCalls int
}

func (q *queryEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	q.queryCalls++
	return []float32{7, 7}, nil
}

func TestEmbedDocumentsFallback(t *testing.T) {
	p := &plainEmbedder{}
	vecs, err := EmbedDocuments(context.Background(), p, []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedDocuments: %v", err)
	}
	if len(vecs) != 2 || p.lastLen != 2 {
		t.Fatalf("fallback should use Embed with both texts: %v len=%d", vecs, p.lastLen)
	}
}

func TestEmbedDocumentsPrefersInterface(t *testing.T) {
	d := &docEmbedder{}
	vecs, err := EmbedDocuments(context.Background(), d, []string{"a"})
	if err != nil {
		t.Fatalf("EmbedDocuments: %v", err)
	}
	if d.docCalls != 1 || d.n != 0 {
		t.Fatalf("docCalls=%d plainCalls=%d, want 1/0", d.docCalls, d.n)
	}
	if vecs[0][0] != 9 {
		t.Fatalf("should use document-prefixed path, got %v", vecs)
	}
}

func TestEmbedQueryFallback(t *testing.T) {
	p := &plainEmbedder{vecs: [][]float32{{3, 4}}}
	vec, err := EmbedQuery(context.Background(), p, "q")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(vec) != 2 || vec[0] != 3 {
		t.Fatalf("unexpected vector: %v", vec)
	}
}

func TestEmbedQueryPrefersInterface(t *testing.T) {
	q := &queryEmbedder{}
	vec, err := EmbedQuery(context.Background(), q, "q")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if q.queryCalls != 1 || len(vec) != 2 || vec[0] != 7 {
		t.Fatalf("query path not used: calls=%d vec=%v", q.queryCalls, vec)
	}
}

func TestEmbedQueryRejectsWrongCount(t *testing.T) {
	p := &plainEmbedder{vecs: [][]float32{{1, 2}, {3, 4}}}
	if _, err := EmbedQuery(context.Background(), p, "q"); err == nil {
		t.Fatal("expected error when embedder returns 2 vectors for 1 query")
	}
}

func TestEmbedQueryPropagatesError(t *testing.T) {
	p := &plainEmbedder{err: errors.New("boom")}
	if _, err := EmbedQuery(context.Background(), p, "q"); err == nil {
		t.Fatal("expected error")
	}
}
