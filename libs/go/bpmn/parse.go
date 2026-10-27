package bpmn

import (
	"errors"
	"io"

	"github.com/indr622/workflow-engine/libs/go/bpmn/internal/xmltree"
)

// Namespace yang dikenali.
const (
	NamespaceBPMN     = "http://www.omg.org/spec/BPMN/20100524/MODEL"
	NamespaceZeebe    = "http://camunda.org/schema/zeebe/1.0"
	NamespaceModeler  = "http://camunda.org/schema/modeler/1.0"
	NamespaceCamunda7 = "http://camunda.org/schema/1.0/bpmn"
)

// Batas default input.
const (
	DefaultMaxBytes    int64 = 4 << 20
	DefaultMaxElements       = 50_000
	DefaultMaxDepth          = 64
)

type parseConfig struct {
	limits xmltree.Limits
}

// ParseOption mengatur batas input Parse.
type ParseOption func(*parseConfig)

// WithMaxBytes membatasi ukuran resource dalam byte.
func WithMaxBytes(n int64) ParseOption { return func(c *parseConfig) { c.limits.MaxBytes = n } }

// WithMaxElements membatasi jumlah elemen XML (di luar diagram interchange).
func WithMaxElements(n int) ParseOption { return func(c *parseConfig) { c.limits.MaxElements = n } }

// WithMaxDepth membatasi kedalaman nesting elemen XML.
func WithMaxDepth(n int) ParseOption { return func(c *parseConfig) { c.limits.MaxDepth = n } }

// Parse membaca satu resource BPMN. Error (*ParseError) hanya untuk kegagalan fatal;
// masalah semantik dilaporkan oleh Validate.
func Parse(r io.Reader, opts ...ParseOption) (*Definitions, error) {
	cfg := parseConfig{limits: xmltree.Limits{
		MaxBytes:    DefaultMaxBytes,
		MaxElements: DefaultMaxElements,
		MaxDepth:    DefaultMaxDepth,
	}}
	for _, o := range opts {
		o(&cfg)
	}

	root, err := xmltree.Parse(r, cfg.limits)
	if err != nil {
		return nil, toParseError(err)
	}
	if root.Space != NamespaceBPMN || root.Local != "definitions" {
		return nil, &ParseError{
			Code:     CodeNotBPMN,
			Message:  "root element must be definitions in the BPMN 2.0 MODEL namespace",
			Position: Position{Line: root.Line, Column: root.Column},
		}
	}
	return build(root), nil
}

// ParseAndValidate menjalankan Parse lalu Validate. Kegagalan fatal juga dikembalikan sebagai
// satu Issue agar pemanggil bisa menampilkan semuanya dengan cara yang sama.
func ParseAndValidate(r io.Reader, p Profile, opts ...ParseOption) (*Definitions, Issues, error) {
	d, err := Parse(r, opts...)
	if err != nil {
		var pe *ParseError
		if errors.As(err, &pe) {
			return nil, Issues{pe.Issue()}, err
		}
		return nil, nil, err
	}
	return d, Validate(d, p), nil
}

func toParseError(err error) error {
	var xe *xmltree.Error
	if !errors.As(err, &xe) {
		return &ParseError{Code: CodeXMLMalformed, Message: err.Error()}
	}
	code := CodeXMLMalformed
	switch xe.Kind {
	case xmltree.TooLarge:
		code = CodeResourceTooLarge
	case xmltree.TooManyElements:
		code = CodeTooManyElements
	case xmltree.TooDeep:
		code = CodeNestingTooDeep
	}
	return &ParseError{Code: code, Message: xe.Err.Error(), Position: Position{Line: xe.Line, Column: xe.Column}}
}
