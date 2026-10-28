package paging

import "testing"

func TestRoundTrip(t *testing.T) {
	for _, tok := range []Token{{Key: "0190f5d2-6c1a-7b3e-8f00-123456789abc"}, {Sequence: 42}} {
		got, err := Decode(Encode(tok))
		if err != nil || got != tok {
			t.Fatalf("round trip %+v = %+v, %v", tok, got, err)
		}
	}
	if Encode(Token{}) != "" {
		t.Fatal("empty token must encode to an empty string")
	}
	if tok, err := Decode(""); err != nil || tok != (Token{}) {
		t.Fatal("empty string is the first page")
	}
	for _, bad := range []string{"!!!", "bm90IGpzb24", Encode(Token{Sequence: 1})[:3]} {
		if _, err := Decode(bad); err == nil {
			t.Errorf("Decode(%q) accepted", bad)
		}
	}
}
