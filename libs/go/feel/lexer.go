package feel

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokString
	tokName
	tokLParen
	tokRParen
	tokDot
	tokComma
	tokEq
	tokNeq
	tokLt
	tokLe
	tokGt
	tokGe
	tokMinus
	// tokOp adalah operator/tanda baca FEEL yang sah tetapi di luar subset (+ * / ** [ ] { } : .. ` ? @).
	tokOp
)

type token struct {
	kind tokenKind
	pos  int
	// text adalah teks mentah token; untuk tokString berisi nilai yang sudah di-unescape.
	text string
}

func (t token) describe() string {
	switch t.kind {
	case tokEOF:
		return "end of expression"
	case tokString:
		return "string literal"
	case tokNumber:
		return "number " + t.text
	case tokName:
		return "name " + t.text
	default:
		return "'" + t.text + "'"
	}
}

func isCompare(k tokenKind) bool {
	return k >= tokEq && k <= tokGe
}

// unsupportedKeywords adalah kata kunci FEEL penuh yang menandai konstruksi di luar subset.
var unsupportedKeywords = map[string]string{
	"in":        "'in' operator",
	"between":   "'between' operator",
	"instance":  "'instance of' operator",
	"of":        "'instance of' operator",
	"if":        "if-then-else expression",
	"then":      "if-then-else expression",
	"else":      "if-then-else expression",
	"some":      "quantified expression (some/every)",
	"every":     "quantified expression (some/every)",
	"satisfies": "quantified expression (some/every)",
	"for":       "for expression",
	"return":    "for expression",
	"function":  "function definition",
	"external":  "function definition",
}

type lexer struct {
	src string
	pos int
}

func isNameStart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r)
}

func isNamePart(r rune) bool {
	return isNameStart(r) || unicode.IsDigit(r)
}

// backslash adalah karakter escape string FEEL.
const backslash = 0x5C

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if r == utf8.RuneError && size <= 1 {
			return token{}, syntaxErr(l.pos, "invalid UTF-8 encoding")
		}
		if !unicode.IsSpace(r) {
			break
		}
		l.pos += size
	}
	start := l.pos
	if start >= len(l.src) {
		return token{kind: tokEOF, pos: start}, nil
	}
	c := l.src[start]
	two := ""
	if start+1 < len(l.src) {
		two = l.src[start : start+2]
	}
	simple := func(k tokenKind, n int) (token, error) {
		l.pos += n
		return token{kind: k, pos: start, text: l.src[start:l.pos]}, nil
	}
	switch {
	case c == '(':
		return simple(tokLParen, 1)
	case c == ')':
		return simple(tokRParen, 1)
	case c == ',':
		return simple(tokComma, 1)
	case two == "..":
		return simple(tokOp, 2)
	case c == '.' && start+1 < len(l.src) && isDigit(l.src[start+1]):
		return l.number()
	case c == '.':
		return simple(tokDot, 1)
	case c == '=':
		return simple(tokEq, 1)
	case two == "!=":
		return simple(tokNeq, 2)
	case two == "<=":
		return simple(tokLe, 2)
	case c == '<':
		return simple(tokLt, 1)
	case two == ">=":
		return simple(tokGe, 2)
	case c == '>':
		return simple(tokGt, 1)
	case c == '-':
		return simple(tokMinus, 1)
	case two == "**":
		return simple(tokOp, 2)
	case strings.IndexByte("+*/[]{}:?@`", c) >= 0:
		return simple(tokOp, 1)
	case c == '"':
		return l.str()
	case isDigit(c):
		return l.number()
	}
	r, size := utf8.DecodeRuneInString(l.src[start:])
	if isNameStart(r) {
		l.pos += size
		for l.pos < len(l.src) {
			r, size = utf8.DecodeRuneInString(l.src[l.pos:])
			if !isNamePart(r) {
				break
			}
			l.pos += size
		}
		return token{kind: tokName, pos: start, text: l.src[start:l.pos]}, nil
	}
	return token{}, syntaxErr(start, "unexpected character %q", r)
}

// number membaca literal angka FEEL: digits [ "." digits ] atau "." digits. Eksponen tidak ada di FEEL.
func (l *lexer) number() (token, error) {
	start := l.pos
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	if l.pos+1 < len(l.src) && l.src[l.pos] == '.' && isDigit(l.src[l.pos+1]) {
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	if l.pos < len(l.src) {
		r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
		if isNamePart(r) {
			return token{}, syntaxErr(start, "invalid number literal")
		}
	}
	return token{kind: tokNumber, pos: start, text: l.src[start:l.pos]}, nil
}

// str membaca string literal berkutip ganda dengan escape FEEL: kutip ganda, kutip tunggal, backslash, n, r, t, dan uXXXX.
func (l *lexer) str() (token, error) {
	start := l.pos
	l.pos++ // kutip pembuka
	var b strings.Builder
	for {
		if l.pos >= len(l.src) {
			return token{}, syntaxErr(start, "unterminated string literal")
		}
		c := l.src[l.pos]
		switch c {
		case '"':
			l.pos++
			return token{kind: tokString, pos: start, text: b.String()}, nil
		case backslash:
			if err := l.escape(&b); err != nil {
				return token{}, err
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if r == utf8.RuneError && size <= 1 {
			return token{}, syntaxErr(l.pos, "invalid UTF-8 encoding")
		}
		b.WriteString(l.src[l.pos : l.pos+size])
		l.pos += size
	}
}

func (l *lexer) escape(b *strings.Builder) error {
	at := l.pos
	if at+1 >= len(l.src) {
		return syntaxErr(at, "unterminated string literal")
	}
	switch e := l.src[at+1]; e {
	case '"', '\'', backslash:
		b.WriteByte(e)
	case 'n':
		b.WriteByte('\n')
	case 'r':
		b.WriteByte('\r')
	case 't':
		b.WriteByte('\t')
	case 'u':
		r, ok := l.hex4(at + 2)
		if !ok {
			return syntaxErr(at, "invalid unicode escape, expected %cuXXXX", backslash)
		}
		l.pos = at + 6
		if utf16IsHigh(r) {
			lo, ok := l.hex4(l.pos + 2)
			if ok && l.pos+1 < len(l.src) && l.src[l.pos] == backslash && l.src[l.pos+1] == 'u' && utf16IsLow(lo) {
				b.WriteRune(0x10000 + (r-0xD800)<<10 + (lo - 0xDC00))
				l.pos += 6
				return nil
			}
		}
		if utf16IsHigh(r) || utf16IsLow(r) {
			// FEEL (Java) menyimpan surrogate tunggal apa adanya; string Go/UTF-8 tidak bisa.
			return unsupportedErr(at, "unpaired surrogate in unicode escape")
		}
		b.WriteRune(r)
		return nil
	default:
		// verify: Camunda 8.8 — perilaku escape lain (mis. \U, \b) di feel-scala belum dipastikan.
		return unsupportedErr(at, "string escape %c%c", backslash, rune(e))
	}
	l.pos = at + 2
	return nil
}

func (l *lexer) hex4(at int) (rune, bool) {
	if at+4 > len(l.src) {
		return 0, false
	}
	var r rune
	for i := at; i < at+4; i++ {
		c := l.src[i]
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(d)
	}
	return r, true
}

func utf16IsHigh(r rune) bool { return r >= 0xD800 && r <= 0xDBFF }
func utf16IsLow(r rune) bool  { return r >= 0xDC00 && r <= 0xDFFF }
