// Package xmltree membaca dokumen XML menjadi pohon node ringan dengan namespace ter-resolve
// dan posisi baris/kolom. Hanya memakai encoding/xml: tidak memproses DTD maupun entity eksternal.
package xmltree

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Namespace yang dilewati seluruh subtree-nya (diagram interchange).
var skippedNamespaces = map[string]bool{
	"http://www.omg.org/spec/BPMN/20100524/DI": true,
	"http://www.omg.org/spec/DD/20100524/DC":   true,
	"http://www.omg.org/spec/DD/20100524/DI":   true,
}

// Node adalah satu elemen XML.
type Node struct {
	Space    string // URI namespace
	Local    string
	Attrs    []xml.Attr
	Children []*Node
	Text     string // gabungan character data langsung di elemen ini
	Line     int
	Column   int
}

// Attr mengembalikan nilai atribut. space kosong berarti atribut tanpa prefix.
func (n *Node) Attr(space, local string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// AttrOr mengembalikan nilai atribut atau def jika tidak ada.
func (n *Node) AttrOr(space, local, def string) string {
	if v, ok := n.Attr(space, local); ok {
		return v
	}
	return def
}

// Child mengembalikan anak pertama dengan nama tersebut.
func (n *Node) Child(space, local string) (*Node, bool) {
	for _, c := range n.Children {
		if c.Space == space && c.Local == local {
			return c, true
		}
	}
	return nil, false
}

// ChildrenNamed mengembalikan semua anak dengan nama tersebut.
func (n *Node) ChildrenNamed(space, local string) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Space == space && c.Local == local {
			out = append(out, c)
		}
	}
	return out
}

// HasAttrInNamespace mengembalikan true jika ada atribut di namespace tersebut.
func (n *Node) HasAttrInNamespace(space string) bool {
	for _, a := range n.Attrs {
		if a.Name.Space == space {
			return true
		}
	}
	return false
}

// Limits membatasi input agar aman dari dokumen berbahaya.
type Limits struct {
	MaxBytes    int64
	MaxElements int
	MaxDepth    int
}

// ErrorKind mengelompokkan kegagalan fatal saat membaca.
type ErrorKind int

const (
	// Malformed berarti XML tidak well-formed.
	Malformed ErrorKind = iota + 1
	// TooLarge berarti ukuran melewati MaxBytes.
	TooLarge
	// TooManyElements berarti jumlah elemen melewati MaxElements.
	TooManyElements
	// TooDeep berarti kedalaman melewati MaxDepth.
	TooDeep
)

// Error adalah kegagalan fatal saat membaca dokumen.
type Error struct {
	Kind   ErrorKind
	Line   int
	Column int
	Err    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("xml %d:%d: %v", e.Line, e.Column, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// countingReader menghitung byte yang dibaca dan berhenti setelah batas+1 byte.
type countingReader struct {
	r     io.Reader
	n     int64
	limit int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n > c.limit {
		return 0, io.EOF
	}
	if rem := c.limit + 1 - c.n; int64(len(p)) > rem {
		p = p[:rem]
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// Parse membaca seluruh dokumen dan mengembalikan elemen root.
func Parse(r io.Reader, lim Limits) (*Node, error) {
	cr := &countingReader{r: r, limit: lim.MaxBytes}
	dec := xml.NewDecoder(cr)
	dec.Strict = true

	var (
		root     *Node
		stack    []*Node
		elements int
		text     []bytes.Buffer
	)
	fail := func(kind ErrorKind, err error) error {
		line, col := dec.InputPos()
		return &Error{Kind: kind, Line: line, Column: col, Err: err}
	}

	for {
		line, col := dec.InputPos()
		tok, err := dec.Token()
		if cr.n > lim.MaxBytes {
			return nil, fail(TooLarge, fmt.Errorf("resource exceeds %d bytes", lim.MaxBytes))
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fail(Malformed, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if skippedNamespaces[t.Name.Space] {
				if err := dec.Skip(); err != nil {
					return nil, fail(Malformed, err)
				}
				continue
			}
			if root != nil && len(stack) == 0 {
				return nil, fail(Malformed, errors.New("multiple root elements"))
			}
			elements++
			if lim.MaxElements > 0 && elements > lim.MaxElements {
				return nil, fail(TooManyElements, fmt.Errorf("more than %d elements", lim.MaxElements))
			}
			if lim.MaxDepth > 0 && len(stack) >= lim.MaxDepth {
				return nil, fail(TooDeep, fmt.Errorf("nesting deeper than %d", lim.MaxDepth))
			}
			n := &Node{
				Space:  t.Name.Space,
				Local:  t.Name.Local,
				Attrs:  append([]xml.Attr(nil), t.Attr...),
				Line:   line,
				Column: col,
			}
			if len(stack) == 0 {
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, n)
			}
			stack = append(stack, n)
			text = append(text, bytes.Buffer{})
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fail(Malformed, errors.New("unexpected end element"))
			}
			top := stack[len(stack)-1]
			top.Text = strings.TrimSpace(text[len(text)-1].String())
			stack = stack[:len(stack)-1]
			text = text[:len(text)-1]
		case xml.CharData:
			if len(text) > 0 {
				_, _ = text[len(text)-1].Write(t) // bytes.Buffer.Write tidak pernah gagal
			}
		case xml.Directive:
			// DOCTYPE dan deklarasi lain ditolak: BPMN tidak memakainya dan berisiko (entity).
			return nil, fail(Malformed, errors.New("XML directives (DOCTYPE) are not allowed"))
		}
	}
	if root == nil {
		return nil, &Error{Kind: Malformed, Line: 1, Column: 1, Err: errors.New("empty document")}
	}
	if len(stack) != 0 {
		return nil, fail(Malformed, errors.New("unexpected end of document"))
	}
	return root, nil
}
