package feel

import (
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
)

// testVars adalah variabel bersama untuk tabel evaluasi.
const testVars = `{
	"a": 1, "b": 2, "s": "abc", "t": true, "f": false, "n": null,
	"big": 9007199254740993, "bigMinus1": 9007199254740992, "dec": 0.1, "decStr": "0.1",
	"price": 10.50, "zero": 0, "neg": -3,
	"order": {
		"total": 150.5,
		"status": "APPROVED",
		"customer": {"name": "Budi", "vip": true, "address": {"city": "Jakarta"}},
		"items": [{"sku": "A", "qty": 1}, {"sku": "B"}],
		"tags": ["x", "y"]
	},
	"list": [1, 2, 3], "list2": [1, 2, 3], "list3": [1, 2], "mixedList": [1, "a"],
	"ctx1": {"x": 1, "y": "a"}, "ctx2": {"y": "a", "x": 1.0}, "ctx3": {"x": 2, "y": "a"},
	"ctxNull": {"x": null}, "ctxMixed": {"x": "1"},
	"emptyStr": "", "emptyList": [], "emptyCtx": {},
	"$dollar": 7, "_under": 8, "näme": "unicode", "camelCase1": 9,
	"objWithList": [{"v": 1}, 2]
}`

func testContext(t *testing.T) Context {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(testVars), &raw); err != nil {
		t.Fatal(err)
	}
	ctx, err := JSONContext(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func mustJSON(t *testing.T, s string) Value {
	t.Helper()
	v, err := FromJSON([]byte(s))
	if err != nil {
		t.Fatalf("FromJSON(%s): %v", s, err)
	}
	return v
}

// identical membandingkan struktur nilai secara persis (berbeda dari "=" FEEL: null identik null).
func identical(a, b Value) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case KindNull:
		return true
	case KindBool:
		return a.b == b.b
	case KindNumber:
		return a.n.Cmp(b.n) == 0
	case KindString:
		return a.s == b.s
	case KindContext:
		if len(a.fields) != len(b.fields) {
			return false
		}
		for k, v := range a.fields {
			w, ok := b.fields[k]
			if !ok || !identical(v, w) {
				return false
			}
		}
		return true
	case KindList:
		if len(a.items) != len(b.items) {
			return false
		}
		for i := range a.items {
			if !identical(a.items[i], b.items[i]) {
				return false
			}
		}
		return true
	}
	return false
}

type evalCase struct {
	src  string
	want string // nilai yang diharapkan dalam JSON
}

func TestEvaluate(t *testing.T) {
	cases := []evalCase{
		// Literal.
		{`true`, `true`},
		{`false`, `false`},
		{`null`, `null`},
		{`42`, `42`},
		{`-5`, `-5`},
		{`- 5`, `-5`},
		{`1.5`, `1.5`},
		{`.5`, `0.5`},
		{`007`, `7`},
		{`"hello"`, `"hello"`},
		{`""`, `""`},
		{`(true)`, `true`},
		{`((("x")))`, `"x"`},
		{"  \t\n 1 \n", `1`},

		// String escape.
		{`"a\"b"`, `"a\"b"`},
		{`"a\\b"`, `"a\\b"`},
		{`"a\nb"`, `"a\nb"`},
		{`"a\rb\tc"`, `"a\rb\tc"`},
		{`"Aé"`, `"Aé"`},
		{`"😀"`, `"😀"`},
		{`"it\'s"`, `"it's"`},
		{`"é😀"`, `"é😀"`},
		{"\"multi\nline\"", `"multi\nline"`}, // verify: Camunda 8.8 — newline mentah di string literal.

		// Path.
		{`a`, `1`},
		{`s`, `"abc"`},
		{`n`, `null`},
		{`missing`, `null`},
		{`order.total`, `150.5`},
		{`order.customer.name`, `"Budi"`},
		{`order.customer.address.city`, `"Jakarta"`},
		{`order.missing`, `null`},
		{`order.customer.missing.deeper`, `null`},
		{`missing.field`, `null`},
		{`s.length`, `null`},       // path melewati string → null.
		{`a.b`, `null`},            // path melewati number → null.
		{`n.x`, `null`},            // path melewati null → null.
		{`order . total`, `150.5`}, // spasi di sekitar titik diperbolehkan FEEL.
		{`$dollar`, `7`},
		{`_under`, `8`},
		{`näme`, `"unicode"`},
		{`camelCase1`, `9`},
		{`order.items.sku`, `["A","B"]`}, // verify: Camunda 8.8 — proyeksi path pada list.
		{`order.items.qty`, `[1,null]`},  // verify: Camunda 8.8 — field hilang di elemen list → null.
		{`objWithList.v`, `[1,null]`},    // verify: Camunda 8.8 — elemen non-context di list → null.
		{`order.tags`, `["x","y"]`},
		{`emptyCtx`, `{}`},

		// Perbandingan angka (desimal eksak).
		{`a = 1`, `true`},
		{`a = 1.0`, `true`},
		{`a != 1`, `false`},
		{`a < b`, `true`},
		{`a <= 1`, `true`},
		{`a > b`, `false`},
		{`b >= 2`, `true`},
		{`order.total > 100`, `true`},
		{`order.total >= 150.50`, `true`},
		{`price = 10.5`, `true`},
		{`dec = 0.1`, `true`},
		{`dec < 0.10000000000000001`, `true`},
		{`big = 9007199254740993`, `true`},
		{`big = bigMinus1`, `false`},
		{`big > bigMinus1`, `true`},
		{`big = 9007199254740992`, `false`},
		{`neg < -2.5`, `true`},
		{`zero = -0`, `true`},
		{`123456789012345678901234567890.123 > 123456789012345678901234567890.122`, `true`}, // verify: Camunda 8.8 — presisi di atas 34 digit (DECIMAL128).

		// Perbandingan string (urutan code point).
		{`s = "abc"`, `true`},
		{`s != "abd"`, `true`},
		{`"a" < "b"`, `true`},
		{`"B" < "a"`, `true`},
		{`"abc" < "abcd"`, `true`},
		{`emptyStr < "a"`, `true`},
		{`order.status = "APPROVED"`, `true`},
		{`"é" > "z"`, `true`},
		{`"￿" < "😀"`, `true`},    // verify: Camunda 8.8 — Java membandingkan UTF-16 (hasil false).
		{`decStr = dec`, `null`}, // string vs number → null.

		// Boolean.
		{`t = true`, `true`},
		{`t != f`, `true`},
		{`t < f`, `null`}, // boolean tidak terurut.
		{`order.customer.vip = true`, `true`},

		// Semantik null.
		{`null = null`, `true`},
		{`n = null`, `true`},
		{`missing = null`, `true`},
		{`a = null`, `false`},
		{`null = a`, `false`},
		{`a != null`, `true`},
		{`missing != null`, `false`},
		{`ctxNull.x = null`, `true`},
		{`n < 1`, `null`},
		{`1 >= n`, `null`},
		{`missing > 0`, `null`},
		{`null < null`, `null`},
		{`null <= null`, `null`}, // verify: Camunda 8.8 — urutan null vs null.

		// Lintas tipe.
		{`1 = "1"`, `null`},  // verify: Camunda 8.8 — "=" lintas tipe → null.
		{`1 != "1"`, `null`}, // verify: Camunda 8.8 — "!=" lintas tipe → null.
		{`true = 1`, `null`}, // verify: Camunda 8.8
		{`"a" < 1`, `null`},
		{`order = "x"`, `null`}, // verify: Camunda 8.8
		{`list = ctx1`, `null`}, // verify: Camunda 8.8

		// Kesetaraan context dan list.
		{`list = list2`, `true`},
		{`list = list3`, `false`},
		{`list != list3`, `true`},
		{`ctx1 = ctx2`, `true`},       // verify: Camunda 8.8 — 1 = 1.0 di dalam context.
		{`ctx1 = ctx3`, `false`},      // verify: Camunda 8.8
		{`ctx1 = ctxMixed`, `false`},  // verify: Camunda 8.8 — kunci berbeda.
		{`mixedList = list3`, `null`}, // verify: Camunda 8.8 — elemen tak terbandingkan.
		{`emptyList = emptyList`, `true`},
		{`list < list2`, `null`},

		// Logika tiga nilai.
		{`true and true`, `true`},
		{`true and false`, `false`},
		{`false and null`, `false`},
		{`null and false`, `false`},
		{`true and null`, `null`},
		{`null and null`, `null`},
		{`true or null`, `true`},
		{`null or true`, `true`},
		{`false or null`, `null`},
		{`false or false`, `false`},
		{`true and "x"`, `null`}, // verify: Camunda 8.8 — operand non-boolean → null.
		{`false or 1`, `null`},   // verify: Camunda 8.8
		{`false and "x"`, `false`},
		{`a = 1 and s = "abc"`, `true`},
		{`a = 2 or s = "abc"`, `true`},
		{`a = 2 or b = 3 or t`, `true`},
		{`true or false and false`, `true`}, // and mengikat lebih kuat daripada or.
		{`(true or false) and false`, `false`},
		{`a > 0 and missing > 0`, `null`},
		{`a > 5 and missing > 0`, `false`},

		// not().
		{`not(true)`, `false`},
		{`not(false)`, `true`},
		{`not(null)`, `null`},
		{`not(1)`, `null`}, // verify: Camunda 8.8 — not() pada non-boolean → null.
		{`not(a = 1)`, `false`},
		{`not(not(t))`, `true`},
		{`not(missing)`, `null`},
		{`not(t) = false`, `true`},
		{`not (true)`, `false`},
	}
	ctx := testContext(t)
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			e, err := Compile(tc.src)
			if err != nil {
				t.Fatalf("Compile(%q): %v", tc.src, err)
			}
			got, err := e.Evaluate(ctx)
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tc.src, err)
			}
			want := mustJSON(t, tc.want)
			if !identical(got, want) {
				t.Fatalf("%s = %#v, want %#v", tc.src, got, want)
			}
		})
	}
}

type errCase struct {
	src         string
	unsupported bool // true: *UnsupportedError, false: *SyntaxError
	pos         int
}

func TestCompileErrors(t *testing.T) {
	cases := []errCase{
		// Di luar subset → UnsupportedError.
		{`a + 1`, true, 2},
		{`0.1 + 0.2`, true, 4},
		{`a - 1`, true, 2},
		{`a * 2`, true, 2},
		{`a / 2`, true, 2},
		{`a ** 2`, true, 2},
		{`-a`, true, 0},
		{`count(x)`, true, 0},
		{`a.b(x)`, true, 0},
		{`date and time("2020-01-01T00:00:00")`, true, 9},
		{`(a)(1)`, true, 0},
		{`x in [1,2]`, true, 2},
		{`x between 1 and 2`, true, 2},
		{`x instance of number`, true, 2},
		{`if a then b else c`, true, 0},
		{`some x in xs satisfies x > 1`, true, 0},
		{`every x in xs satisfies x > 1`, true, 0},
		{`for x in xs return x`, true, 0},
		{`function(x) x`, true, 0},
		{`[1, 2]`, true, 0},
		{`{a: 1}`, true, 0},
		{`items[1]`, true, 5},
		{`items[x > 1]`, true, 5},
		{`@"2020-01-01"`, true, 0},
		{"`name with space`", true, 0},
		{`?`, true, 0},
		{`1..3`, true, 1},
		{`a = b = c`, true, 6},
		{`not(a, b)`, true, 0},
		{`not(negand: a)`, true, 4},
		{`(a).b`, true, 3},
		{`"abc".length`, true, 5},
		{`a.in`, true, 2},
		{`a > 1 // comment`, true, 6},
		{`a /* c */ > 1`, true, 2},
		{`"\U0001F600"`, true, 1}, // verify: Camunda 8.8 — escape \U.
		{`"\x"`, true, 1},
		{`"\uD800"`, true, 1},
		{`- (1)`, true, 0},

		// Sintaks rusak → SyntaxError.
		{``, false, 0},
		{`   `, false, 3},
		{`(`, false, 1},
		{`(a`, false, 2},
		{`a)`, false, 1},
		{`a =`, false, 3},
		{`= a`, false, 0},
		{`a and`, false, 5},
		{`or a`, false, 0},
		{`a.`, false, 2},
		{`a..b`, true, 1},
		{`.a`, false, 0},
		{`1a`, false, 0},
		{`1e5`, false, 0},
		{`"abc`, false, 0},
		{`"ab\`, false, 3},
		{`"\u12"`, false, 1},
		{`'abc'`, false, 0},
		{`a ! b`, false, 2},
		{`a # b`, false, 2},
		{`not true`, false, 0},
		{`not`, false, 0},
		{`not(a`, false, 5},
		{`a b`, false, 2},
		{`a.true`, false, 2},
		{`a.1`, false, 1},
		{`true.x`, true, 4},
		{`1 2`, false, 2},
		{`a, b`, false, 1},
		{"a\xff", false, 1},
		{`5(1)`, false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			e, err := Compile(tc.src)
			if err == nil {
				t.Fatalf("Compile(%q) succeeded: %#v", tc.src, e)
			}
			var se *SyntaxError
			var ue *UnsupportedError
			switch {
			case tc.unsupported && errors.As(err, &ue):
				if ue.Pos != tc.pos {
					t.Fatalf("Compile(%q): pos %d, want %d (%v)", tc.src, ue.Pos, tc.pos, err)
				}
			case !tc.unsupported && errors.As(err, &se):
				if se.Pos != tc.pos {
					t.Fatalf("Compile(%q): pos %d, want %d (%v)", tc.src, se.Pos, tc.pos, err)
				}
			default:
				t.Fatalf("Compile(%q): wrong error type %T: %v", tc.src, err, err)
			}
			if !strings.HasPrefix(err.Error(), "feel: ") {
				t.Fatalf("error message %q lacks prefix", err.Error())
			}
		})
	}
}

func TestDeepNestingIsRejected(t *testing.T) {
	src := strings.Repeat("(", maxDepth+1) + "1" + strings.Repeat(")", maxDepth+1)
	_, err := Compile(src)
	var ue *UnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("want *UnsupportedError, got %v", err)
	}
	ok := strings.Repeat("(", maxDepth-1) + "1" + strings.Repeat(")", maxDepth-1)
	if _, err := Compile(ok); err != nil {
		t.Fatalf("nesting %d: %v", maxDepth-1, err)
	}
}

func TestEvaluateNilContext(t *testing.T) {
	e, err := Compile(`x = null and not(y)`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Evaluate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsNull() {
		t.Fatalf("got %#v, want null", v)
	}
}

func TestSourceAndShape(t *testing.T) {
	for src, want := range map[string]bool{
		`"payment"`: true, `jobType`: true, `order.type`: true, `3`: true, `-3`: true, `null`: true,
		`a = 1`: false, `not(a)`: false, `(a)`: true, `a or b`: false,
	} {
		e, err := Compile(src)
		if err != nil {
			t.Fatal(err)
		}
		if e.Source() != src {
			t.Errorf("Source() = %q, want %q", e.Source(), src)
		}
		if got := e.IsLiteralOrPath(); got != want {
			t.Errorf("IsLiteralOrPath(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestStripPrefix(t *testing.T) {
	cases := []struct {
		raw, src string
		isFEEL   bool
	}{
		{`=a > 1`, `a > 1`, true},
		{`="payment"`, `"payment"`, true},
		{`payment`, `payment`, false},
		{``, ``, false},
		{` =a`, `a`, true}, // verify: Camunda 8.8 — spasi sebelum "=".
		{`a=b`, `a=b`, false},
	}
	for _, tc := range cases {
		src, ok := StripPrefix(tc.raw)
		if src != tc.src || ok != tc.isFEEL {
			t.Errorf("StripPrefix(%q) = %q, %v; want %q, %v", tc.raw, src, ok, tc.src, tc.isFEEL)
		}
	}
}

func TestValueAccessors(t *testing.T) {
	v := mustJSON(t, `{"n": 9007199254740993, "f": 1.25, "s": "x", "b": true, "l": [1], "huge": 1e30, "neg": -7}`)
	if v.Kind() != KindContext {
		t.Fatalf("kind %v", v.Kind())
	}
	n, _ := v.Field("n")
	if i, ok := n.Int64(); !ok || i != 9007199254740993 {
		t.Errorf("Int64 = %d, %v", i, ok)
	}
	f, _ := v.Field("f")
	if _, ok := f.Int64(); ok {
		t.Error("Int64 on 1.25 must fail")
	}
	r, ok := f.Number()
	if !ok || r.Cmp(big.NewRat(5, 4)) != 0 {
		t.Errorf("Number = %v, %v", r, ok)
	}
	r.SetInt64(99) // salinan; tidak boleh mengubah nilai asli.
	if r2, _ := f.Number(); r2.Cmp(big.NewRat(5, 4)) != 0 {
		t.Error("Number must return a copy")
	}
	huge, _ := v.Field("huge")
	if _, ok := huge.Int64(); ok {
		t.Error("Int64 on 1e30 must fail")
	}
	neg, _ := v.Field("neg")
	if i, ok := neg.Int64(); !ok || i != -7 {
		t.Errorf("Int64(neg) = %d, %v", i, ok)
	}
	s, _ := v.Field("s")
	if str, ok := s.String(); !ok || str != "x" {
		t.Errorf("String = %q, %v", str, ok)
	}
	if _, ok := n.String(); ok {
		t.Error("String on number must fail")
	}
	b, _ := v.Field("b")
	if bv, ok := b.Bool(); !ok || !bv {
		t.Errorf("Bool = %v, %v", bv, ok)
	}
	if _, ok := s.Bool(); ok {
		t.Error("Bool on string must fail")
	}
	l, _ := v.Field("l")
	if items, ok := l.Items(); !ok || len(items) != 1 {
		t.Errorf("Items = %v, %v", items, ok)
	}
	if fields, ok := v.Fields(); !ok || len(fields) != 7 {
		t.Errorf("Fields = %d, %v", len(fields), ok)
	}
	if !Null().IsNull() || (Value{}).Kind() != KindNull {
		t.Error("zero Value must be null")
	}
	if !NewNumber(nil).IsNull() {
		t.Error("NewNumber(nil) must be null")
	}
	if got := mustJSON(t, `{"b":[1.5,null,"q"],"a":true}`).GoString(); got != `{a: true, b: [1.5, null, "q"]}` {
		t.Errorf("GoString = %s", got)
	}
}

func TestFromJSONErrors(t *testing.T) {
	for _, raw := range []string{``, `{`, `1 2`, `[1,]`, `1e99999`, `"\x"`} {
		if _, err := FromJSON([]byte(raw)); err == nil {
			t.Errorf("FromJSON(%q) succeeded", raw)
		}
	}
	if _, err := JSONContext(map[string][]byte{"x": []byte(`{`)}); err == nil {
		t.Error("JSONContext with invalid JSON must fail")
	}
}

func TestMapContext(t *testing.T) {
	e, err := Compile(`order.total >= 100 and vip`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := MapContext(map[string]Value{
		"order": NewContext(map[string]Value{"total": NewInt(100)}),
		"vip":   NewBool(true),
	})
	v, _ := e.Evaluate(ctx)
	if b, ok := v.Bool(); !ok || !b {
		t.Fatalf("got %#v", v)
	}
}
