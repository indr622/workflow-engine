package feel

import (
	"errors"
	"testing"
)

// fuzzContexts adalah beberapa konteks dengan bentuk berbeda untuk mengevaluasi ekspresi hasil fuzz.
var fuzzContexts = []string{
	`{}`,
	`{"a": 1, "b": "x", "c": true, "d": null, "e": [1, {"a": 2}], "f": {"a": {"b": 1.5}}}`,
	`{"a": "1", "b": [], "c": {}, "x": 9007199254740993, "y": -0.000001, "order": {"total": 1}}`,
	`{"a": [[{"b": 1}]], "b": {"c": [null, false]}, "s": "é😀", "t": false}`,
}

// FuzzCompile: Compile tidak boleh panic dan hanya mengembalikan *SyntaxError atau
// *UnsupportedError; bila sukses, Evaluate tidak boleh panic maupun error pada konteks apa pun.
func FuzzCompile(f *testing.F) {
	for _, s := range []string{
		`a`, `a.b.c`, `a = 1`, `a != "x"`, `a < 1.5 and b >= -2`, `not(a) or b`, `(a or b) and c`,
		`"esc\"aped\\\nA😀"`, `null = null`, `9007199254740993 > x`, `order.total > 100`,
		`a + 1`, `count(x)`, `x in [1,2]`, `if a then b else c`, `0.1 + 0.2`, `{a: 1}`, `@"P1D"`,
		`not(negand: a)`, `a = b = c`, `((((((1))))))`, `"unterminated`, `1e5`, `.5`, `- 5`,
		`a.in`, `x between 1 and 2`, "a\xff", `a // c`, `e.a`, `a.b.c.d.e.f`,
	} {
		f.Add(s)
	}
	ctxs := make([]Context, 0, len(fuzzContexts))
	for _, raw := range fuzzContexts {
		v, err := FromJSON([]byte(raw))
		if err != nil {
			f.Fatal(err)
		}
		fields, _ := v.Fields()
		ctxs = append(ctxs, MapContext(fields))
	}
	ctxs = append(ctxs, nil)

	f.Fuzz(func(t *testing.T, src string) {
		e, err := Compile(src)
		if err != nil {
			var se *SyntaxError
			var ue *UnsupportedError
			switch {
			case errors.As(err, &se):
				if se.Pos < 0 || se.Pos > len(src) {
					t.Fatalf("syntax error pos %d out of range for %q", se.Pos, src)
				}
			case errors.As(err, &ue):
				if ue.Pos < 0 || ue.Pos > len(src) {
					t.Fatalf("unsupported error pos %d out of range for %q", ue.Pos, src)
				}
			default:
				t.Fatalf("unexpected error type %T: %v", err, err)
			}
			return
		}
		for _, ctx := range ctxs {
			if _, err := e.Evaluate(ctx); err != nil {
				t.Fatalf("Evaluate(%q): %v", src, err)
			}
		}
	})
}
