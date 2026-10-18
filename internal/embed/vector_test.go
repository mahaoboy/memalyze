package embed

import (
	"math/rand"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := []float32{0, 1, -1, 0.5, -0.25, 1e-8, 3.14159}
	out, err := Decode(Encode(in))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("index %d: got %v want %v", i, out[i], in[i])
		}
	}
}

func TestEncodeIsLittleEndian(t *testing.T) {
	// 1.0 的 IEEE754 位模式是 0x3F800000，小端应为 00 00 80 3F
	b := Encode([]float32{1})
	want := []byte{0x00, 0x00, 0x80, 0x3F}
	for i := range want {
		if b[i] != want[i] {
			t.Fatalf("byte %d: got %#x want %#x", i, b[i], want[i])
		}
	}
}

func TestDecodeRejectsBadLength(t *testing.T) {
	if _, err := Decode([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for length 3, got nil")
	}
	if _, err := Decode(nil); err != nil {
		t.Fatalf("empty blob should decode to empty vector, got %v", err)
	}
}

func TestCosine(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float64
		eps  float64
	}{
		{"same", []float32{1, 2, 3}, []float32{1, 2, 3}, 1, 1e-9},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0, 1e-9},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1, 1e-9},
		{"zero", []float32{0, 0}, []float32{1, 1}, 0, 1e-9},
	}
	for _, c := range cases {
		got, err := Cosine(c.a, c.b)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if diff := got - c.want; diff > c.eps || diff < -c.eps {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCosineRejectsMismatchedDims(t *testing.T) {
	if _, err := Cosine([]float32{1, 2}, []float32{1, 2, 3}); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

func TestTopKOrdering(t *testing.T) {
	query := []float32{1, 0}
	vectors := [][]float32{{0, 1}, {1, 0}, {0.9, 0.1}, {0, 0}}
	idx, scores := TopK(query, vectors, 2)
	if len(idx) != 2 {
		t.Fatalf("want 2 hits, got %d", len(idx))
	}
	if idx[0] != 1 {
		t.Fatalf("expected exact match at index 1 first, got %d", idx[0])
	}
	if scores[0] < scores[1] {
		t.Fatalf("scores not descending: %v", scores)
	}
}

func TestTopKHandlesMismatchedRows(t *testing.T) {
	query := []float32{1, 0}
	vectors := [][]float32{{1, 0, 0}, {1, 0}} // 第一行维度不对，应被跳过
	idx, _ := TopK(query, vectors, 5)
	if len(idx) != 1 || idx[0] != 1 {
		t.Fatalf("expected only index 1, got %v", idx)
	}
}

func BenchmarkCosine768(b *testing.B) {
	const dim = 768
	a := make([]float32, dim)
	c := make([]float32, dim)
	r := rand.New(rand.NewSource(1))
	for i := 0; i < dim; i++ {
		a[i], c[i] = r.Float32(), r.Float32()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Cosine(a, c); err != nil {
			b.Fatal(err)
		}
	}
}
