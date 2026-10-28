package idempotency

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestValidateKey(t *testing.T) {
	for key, ok := range map[string]bool{
		"a":                      true,
		strings.Repeat("x", 128): true,
		"":                       false,
		strings.Repeat("x", 129): false,
		string([]byte{0xff}):     false,
	} {
		if got := ValidateKey(key) == nil; got != ok {
			t.Errorf("ValidateKey(%q) ok=%v, want %v", key, got, ok)
		}
	}
}

func TestHashIgnoresClearedFieldAndIsDeterministic(t *testing.T) {
	msg := func(id, v string) *structpb.Struct {
		s, _ := structpb.NewStruct(map[string]any{"request_id": id, "b": v, "a": 1})
		return s
	}
	clear := func(m proto.Message) { delete(m.(*structpb.Struct).Fields, "request_id") }
	h1, err := Hash(msg("one", "x"), clear)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := Hash(msg("two", "x"), clear)
	h3, _ := Hash(msg("one", "y"), clear)
	if h1 != h2 {
		t.Fatal("hash must ignore the cleared request id")
	}
	if h1 == h3 {
		t.Fatal("hash must change with the payload")
	}
	original := msg("keep", "x")
	if _, err := Hash(original, clear); err != nil {
		t.Fatal(err)
	}
	if _, ok := original.Fields["request_id"]; !ok {
		t.Fatal("Hash must not modify the caller's message")
	}
}
