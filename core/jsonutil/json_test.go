package jsonutil

import (
	"bytes"
	"encoding/json/jsontext"
	"strings"
	"testing"
)

type sample struct {
	A int      `json:"a"`
	B []string `json:"b"`
}

func TestMarshalIndent(t *testing.T) {
	got, err := MarshalIndent(sample{A: 1, B: []string{"x"}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": 1,\n  \"b\": [\n    \"x\"\n  ]\n}"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEncoderDecoderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(sample{A: 1}); err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(sample{A: 2}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "{\"a\":1,\"b\":[]}\n{\"a\":2,\"b\":[]}\n" {
		t.Fatalf("encoded %q", got)
	}

	dec := NewDecoder(strings.NewReader(buf.String()))
	var first, second sample
	if err := dec.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&second); err != nil {
		t.Fatal(err)
	}
	if first.A != 1 || second.A != 2 {
		t.Fatalf("decoded %+v %+v", first, second)
	}
}

func TestRawMessage(t *testing.T) {
	raw := RawMessage(`{"x":1}`)
	if !jsontext.Value(raw).IsValid() {
		t.Fatal("raw value should be valid JSON")
	}
}
