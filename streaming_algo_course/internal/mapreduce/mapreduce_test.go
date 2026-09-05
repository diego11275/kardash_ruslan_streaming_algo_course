package mapreduce

import (
	"bytes"
	"context"
	"testing"

	"kvschool/internal/kv/memmap"
)

func TestRun_WordCount(t *testing.T) {
	ctx := context.Background()
	in := []byte("a b a\nb b\n")
	st := memmap.New()
	out, err := Run(ctx, bytes.NewReader(in), st, WordCountMapper, SumVarintReducer)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = out.Close() })

	get := func(k string) int64 {
		v, err := out.Get(ctx, []byte(k))
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}
		x, err := decodeVarint(v)
		if err != nil {
			t.Fatalf("decodeVarint(%q): %v", k, err)
		}
		return x
	}

	if got := get("a"); got != 2 {
		t.Fatalf("a=%d want=2", got)
	}
	if got := get("b"); got != 3 {
		t.Fatalf("b=%d want=3", got)
	}
}

func TestIntermediateKey_RoundTrip(t *testing.T) {
	k := []byte("hello")
	raw := makeIntermediateKey(k, 42)
	logical, ok := splitIntermediateKey(raw)
	if !ok {
		t.Fatalf("splitIntermediateKey failed")
	}
	if string(logical) != "hello" {
		t.Fatalf("logical=%q", string(logical))
	}
}
