package feel

import (
	"math/big"
	"strings"
)

// maxDepth membatasi kedalaman kurung/not bersarang agar parser dan evaluator tidak menghabiskan stack.
const maxDepth = 128

// Expression adalah ekspresi FEEL yang sudah dikompilasi. Immutable dan aman dipakai bersamaan.
type Expression struct {
	src  string
	root node
}

// Compile mem-parse ekspresi FEEL (tanpa "=" di depan). Ekspresi di luar subset menghasilkan
// *UnsupportedError dengan posisi; sintaks rusak menghasilkan *SyntaxError.
func Compile(src string) (*Expression, error) {
	p := &parser{lex: lexer{src: src}}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.tok.kind == tokEOF {
		return nil, syntaxErr(p.tok.pos, "empty expression")
	}
	root, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.tok.kind != tokEOF {
		return nil, p.unexpected()
	}
	return &Expression{src: src, root: root}, nil
}

// Source mengembalikan sumber ekspresi seperti yang diberikan ke Compile.
func (e *Expression) Source() string { return e.src }

// IsLiteralOrPath melaporkan apakah ekspresi hanya berupa literal atau path variabel (aturan untuk
// zeebe:taskDefinition type/retries).
func (e *Expression) IsLiteralOrPath() bool {
	switch e.root.(type) {
	case literalNode, pathNode:
		return true
	}
	return false
}

// StripPrefix mengembalikan sumber FEEL dari nilai atribut Zeebe "=expr" dan apakah nilai itu FEEL.
// Nilai tanpa "=" di depan adalah literal statis dan dikembalikan apa adanya.
func StripPrefix(raw string) (string, bool) {
	// verify: Camunda 8.8 — apakah Zeebe mengabaikan spasi sebelum "=". Di sini spasi awal diabaikan.
	trimmed := strings.TrimLeft(raw, " \t\r\n")
	if strings.HasPrefix(trimmed, "=") {
		return trimmed[1:], true
	}
	return raw, false
}

type parser struct {
	lex   lexer
	tok   token
	depth int
}

func (p *parser) advance() error {
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.tok = t
	return nil
}

// unexpected membuat error untuk token saat ini yang tidak cocok dengan grammar: operator dan kata
// kunci FEEL penuh menjadi *UnsupportedError, selebihnya *SyntaxError.
func (p *parser) unexpected() error {
	t := p.tok
	switch t.kind {
	case tokMinus, tokOp:
		if t.text == "/" && p.lex.pos < len(p.lex.src) && (p.lex.src[p.lex.pos] == '/' || p.lex.src[p.lex.pos] == '*') {
			return unsupportedErr(t.pos, "comment")
		}
		return unsupportedErr(t.pos, "operator '%s'", t.text)
	case tokName:
		if what, ok := unsupportedKeywords[t.text]; ok {
			return unsupportedErr(t.pos, "%s", what)
		}
	}
	return syntaxErr(t.pos, "unexpected %s", t.describe())
}

func (p *parser) expect(k tokenKind) error {
	if p.tok.kind != k {
		return p.unexpected()
	}
	return p.advance()
}

func (p *parser) parseExpr() (node, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxDepth {
		return nil, unsupportedErr(p.tok.pos, "expression nesting deeper than %d levels", maxDepth)
	}
	return p.parseOr()
}

func (p *parser) isKeyword(kw string) bool {
	return p.tok.kind == tokName && p.tok.text == kw
}

func (p *parser) parseOr() (node, error) {
	first, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	xs := []node{first}
	for p.isKeyword("or") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		x, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		xs = append(xs, x)
	}
	if len(xs) == 1 {
		return first, nil
	}
	return orNode{xs: xs}, nil
}

func (p *parser) parseAnd() (node, error) {
	first, err := p.parseCompare()
	if err != nil {
		return nil, err
	}
	xs := []node{first}
	for p.isKeyword("and") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		x, err := p.parseCompare()
		if err != nil {
			return nil, err
		}
		xs = append(xs, x)
	}
	if len(xs) == 1 {
		return first, nil
	}
	return andNode{xs: xs}, nil
}

func (p *parser) parseCompare() (node, error) {
	l, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if !isCompare(p.tok.kind) {
		return l, nil
	}
	op := p.tok.kind
	if err := p.advance(); err != nil {
		return nil, err
	}
	r, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if isCompare(p.tok.kind) {
		return nil, unsupportedErr(p.tok.pos, "chained comparison")
	}
	return compareNode{op: op, l: l, r: r}, nil
}

func (p *parser) parseUnary() (node, error) {
	start := p.tok.pos
	var n node
	var err error
	callable := false
	if p.isKeyword("not") {
		n, err = p.parseNot()
	} else {
		n, callable, err = p.parsePrimary()
	}
	if err != nil {
		return nil, err
	}
	// Postfix FEEL penuh yang tidak didukung: pemanggilan fungsi, filter, path pada non-variabel.
	switch p.tok.kind {
	case tokLParen:
		if callable {
			return nil, unsupportedErr(start, "function invocation")
		}
		return nil, syntaxErr(p.tok.pos, "unexpected '('")
	case tokOp:
		if p.tok.text == "[" {
			return nil, unsupportedErr(p.tok.pos, "filter expression")
		}
	case tokDot:
		return nil, unsupportedErr(p.tok.pos, "path expression on a non-variable value")
	}
	return n, nil
}

func (p *parser) parseNot() (node, error) {
	notPos := p.tok.pos
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.tok.kind != tokLParen {
		return nil, syntaxErr(notPos, "'not' must be followed by '('")
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.tok.kind == tokName {
		// Parameter bernama, mis. not(negand: x): intip satu token tanpa mengonsumsi.
		peek := p.lex
		if t, err := peek.next(); err == nil && t.kind == tokOp && t.text == ":" {
			return nil, unsupportedErr(p.tok.pos, "named function parameters")
		}
	}
	x, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.tok.kind == tokComma {
		return nil, unsupportedErr(notPos, "not() with more than one argument")
	}
	if err := p.expect(tokRParen); err != nil {
		return nil, err
	}
	return notNode{x: x}, nil
}

// parsePrimary mengembalikan simpul dan apakah simpul itu bisa diikuti pemanggilan fungsi
// (path atau ekspresi berkurung).
func (p *parser) parsePrimary() (node, bool, error) {
	t := p.tok
	switch t.kind {
	case tokNumber:
		n, err := p.numberLiteral(t, false)
		return n, false, err
	case tokMinus:
		if err := p.advance(); err != nil {
			return nil, false, err
		}
		if p.tok.kind != tokNumber {
			return nil, false, unsupportedErr(t.pos, "arithmetic negation")
		}
		n, err := p.numberLiteral(p.tok, true)
		return n, false, err
	case tokString:
		if err := p.advance(); err != nil {
			return nil, false, err
		}
		return literalNode{v: NewString(t.text)}, false, nil
	case tokLParen:
		if err := p.advance(); err != nil {
			return nil, false, err
		}
		x, err := p.parseExpr()
		if err != nil {
			return nil, false, err
		}
		if err := p.expect(tokRParen); err != nil {
			return nil, false, err
		}
		return x, true, nil
	case tokName:
		switch t.text {
		case "true", "false":
			if err := p.advance(); err != nil {
				return nil, false, err
			}
			return literalNode{v: NewBool(t.text == "true")}, false, nil
		case "null":
			if err := p.advance(); err != nil {
				return nil, false, err
			}
			return literalNode{}, false, nil
		}
		n, err := p.parsePath()
		return n, true, err
	case tokOp:
		switch t.text {
		case "[":
			return nil, false, unsupportedErr(t.pos, "list literal")
		case "{":
			return nil, false, unsupportedErr(t.pos, "context literal")
		case "@":
			return nil, false, unsupportedErr(t.pos, "temporal literal")
		case "`":
			return nil, false, unsupportedErr(t.pos, "backtick-quoted name")
		case "?":
			return nil, false, unsupportedErr(t.pos, "unary test input '?'")
		}
	}
	return nil, false, p.unexpected()
}

func (p *parser) numberLiteral(t token, negative bool) (node, error) {
	text := t.text
	if negative {
		text = "-" + text
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, syntaxErr(t.pos, "invalid number literal")
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return literalNode{v: Value{kind: KindNumber, n: r}}, nil
}

func (p *parser) checkName() error {
	t := p.tok
	if t.kind != tokName {
		return p.unexpected()
	}
	switch t.text {
	case "true", "false", "null", "and", "or", "not":
		return syntaxErr(t.pos, "reserved word '%s' cannot be used as a name", t.text)
	}
	if what, ok := unsupportedKeywords[t.text]; ok {
		return unsupportedErr(t.pos, "%s", what)
	}
	return nil
}

func (p *parser) parsePath() (node, error) {
	var names []string
	for {
		if err := p.checkName(); err != nil {
			return nil, err
		}
		names = append(names, p.tok.text)
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind != tokDot {
			return pathNode{names: names}, nil
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
}
