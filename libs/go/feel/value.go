package feel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
)

// Kind adalah tipe nilai FEEL yang dikenal subset ini.
type Kind int

// Konstanta Kind. Diberi prefix "Kind" karena nama Context sudah dipakai interface variabel.
const (
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindContext
	KindList
)

func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindBool:
		return "boolean"
	case KindNumber:
		return "number"
	case KindString:
		return "string"
	case KindContext:
		return "context"
	case KindList:
		return "list"
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

// Value adalah nilai FEEL yang immutable. Zero value adalah null.
type Value struct {
	kind   Kind
	b      bool
	n      *big.Rat
	s      string
	fields map[string]Value
	items  []Value
}

// Null mengembalikan nilai null FEEL (sama dengan zero value Value).
func Null() Value { return Value{} }

// NewBool membuat nilai boolean.
func NewBool(b bool) Value { return Value{kind: KindBool, b: b} }

// NewNumber membuat nilai number dari salinan r; r nil menghasilkan null.
func NewNumber(r *big.Rat) Value {
	if r == nil {
		return Value{}
	}
	return Value{kind: KindNumber, n: new(big.Rat).Set(r)}
}

// NewInt membuat nilai number dari bilangan bulat.
func NewInt(i int64) Value { return Value{kind: KindNumber, n: new(big.Rat).SetInt64(i)} }

// NewString membuat nilai string.
func NewString(s string) Value { return Value{kind: KindString, s: s} }

// NewContext membuat nilai context dari salinan dangkal m.
func NewContext(m map[string]Value) Value {
	fields := make(map[string]Value, len(m))
	for k, v := range m {
		fields[k] = v
	}
	return Value{kind: KindContext, fields: fields}
}

// NewList membuat nilai list dari salinan dangkal items.
func NewList(items []Value) Value {
	return Value{kind: KindList, items: append([]Value(nil), items...)}
}

// Kind mengembalikan tipe nilai.
func (v Value) Kind() Kind { return v.kind }

// IsNull melaporkan apakah nilai adalah null.
func (v Value) IsNull() bool { return v.kind == KindNull }

// Bool mengembalikan nilai boolean; ok false bila nilai bukan boolean.
func (v Value) Bool() (b, ok bool) { return v.b, v.kind == KindBool }

// String mengembalikan isi string; ok false bila nilai bukan string.
//
// Catatan: tanda tangan ini sengaja (ditetapkan rencana Fase 2) dan berbeda dari fmt.Stringer,
// sehingga Value tidak memenuhi fmt.Stringer.
func (v Value) String() (s string, ok bool) { return v.s, v.kind == KindString }

// Number mengembalikan salinan angka eksak; ok false bila nilai bukan number.
func (v Value) Number() (*big.Rat, bool) {
	if v.kind != KindNumber {
		return nil, false
	}
	return new(big.Rat).Set(v.n), true
}

// Int64 mengembalikan angka bila nilai adalah number bulat yang muat di int64.
func (v Value) Int64() (int64, bool) {
	if v.kind != KindNumber || !v.n.IsInt() || !v.n.Num().IsInt64() {
		return 0, false
	}
	return v.n.Num().Int64(), true
}

// Field mengembalikan field context; ok false bila nilai bukan context atau field tidak ada.
func (v Value) Field(name string) (Value, bool) {
	if v.kind != KindContext {
		return Value{}, false
	}
	f, ok := v.fields[name]
	return f, ok
}

// Fields mengembalikan salinan field context; ok false bila nilai bukan context.
func (v Value) Fields() (map[string]Value, bool) {
	if v.kind != KindContext {
		return nil, false
	}
	return NewContext(v.fields).fields, true
}

// Items mengembalikan salinan elemen list; ok false bila nilai bukan list.
func (v Value) Items() ([]Value, bool) {
	if v.kind != KindList {
		return nil, false
	}
	return append([]Value(nil), v.items...), true
}

// GoString menghasilkan representasi mirip FEEL untuk debugging dan pesan uji.
func (v Value) GoString() string {
	var b strings.Builder
	v.format(&b)
	return b.String()
}

func (v Value) format(b *strings.Builder) {
	switch v.kind {
	case KindNull:
		b.WriteString("null")
	case KindBool:
		b.WriteString(strconv.FormatBool(v.b))
	case KindNumber:
		b.WriteString(formatRat(v.n))
	case KindString:
		b.WriteString(strconv.Quote(v.s))
	case KindContext:
		keys := sortedKeys(v.fields)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k)
			b.WriteString(": ")
			v.fields[k].format(b)
		}
		b.WriteByte('}')
	case KindList:
		b.WriteByte('[')
		for i, it := range v.items {
			if i > 0 {
				b.WriteString(", ")
			}
			it.format(b)
		}
		b.WriteByte(']')
	}
}

func sortedKeys(m map[string]Value) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// insertion sort cukup untuk keperluan debug dan menghindari import tambahan.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// formatRat menulis desimal eksak bila penyebut hanya berfaktor 2 dan 5, selain itu a/b.
func formatRat(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	den := new(big.Int).Set(r.Denom())
	mod := new(big.Int)
	for _, f := range []int64{2, 5} {
		bf := big.NewInt(f)
		for {
			q, m := new(big.Int).QuoRem(den, bf, mod)
			if m.Sign() != 0 {
				break
			}
			den = q
		}
	}
	if !den.IsInt64() || den.Int64() != 1 {
		return r.String()
	}
	// Jumlah digit desimal minimum agar r * 10^k bulat.
	scaled := new(big.Rat).Set(r)
	ten := new(big.Rat).SetInt64(10)
	k := 0
	for !scaled.IsInt() {
		scaled.Mul(scaled, ten)
		k++
	}
	return r.FloatString(k)
}

// maxJSONExponent membatasi eksponen angka JSON (rentang desimal128) agar big.Rat tidak meledak.
const maxJSONExponent = 6144

// FromJSON mendekode satu nilai JSON menjadi Value: object menjadi context, array menjadi list,
// angka dibaca eksak (json.Decoder.UseNumber).
func FromJSON(raw []byte) (Value, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return Value{}, fmt.Errorf("feel: decode JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Value{}, errors.New("feel: decode JSON: unexpected data after value")
	}
	return fromAny(x)
}

func fromAny(x any) (Value, error) {
	switch t := x.(type) {
	case nil:
		return Value{}, nil
	case bool:
		return NewBool(t), nil
	case string:
		return NewString(t), nil
	case json.Number:
		return parseJSONNumber(string(t))
	case map[string]any:
		fields := make(map[string]Value, len(t))
		for k, e := range t {
			v, err := fromAny(e)
			if err != nil {
				return Value{}, err
			}
			fields[k] = v
		}
		return Value{kind: KindContext, fields: fields}, nil
	case []any:
		items := make([]Value, len(t))
		for i, e := range t {
			v, err := fromAny(e)
			if err != nil {
				return Value{}, err
			}
			items[i] = v
		}
		return Value{kind: KindList, items: items}, nil
	}
	return Value{}, fmt.Errorf("feel: unsupported JSON value of type %T", x)
}

func parseJSONNumber(s string) (Value, error) {
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exp, err := strconv.Atoi(s[i+1:])
		if err != nil || exp > maxJSONExponent || exp < -maxJSONExponent {
			return Value{}, fmt.Errorf("feel: JSON number %q out of supported range", s)
		}
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return Value{}, fmt.Errorf("feel: invalid JSON number %q", s)
	}
	return Value{kind: KindNumber, n: r}, nil
}

// Context menyediakan variabel untuk evaluasi ekspresi.
type Context interface {
	// Lookup mengembalikan nilai variabel; ok false bila variabel tidak ada (dievaluasi sebagai null).
	Lookup(name string) (Value, bool)
}

type mapContext map[string]Value

func (m mapContext) Lookup(name string) (Value, bool) {
	v, ok := m[name]
	return v, ok
}

// MapContext membungkus map variabel sebagai Context. Map tidak disalin; jangan diubah setelahnya.
func MapContext(m map[string]Value) Context { return mapContext(m) }

// JSONContext mendekode setiap variabel JSON (mis. map[string]json.RawMessage dari engine)
// menjadi Context. Error bila ada variabel yang bukan JSON valid.
func JSONContext[T ~[]byte](vars map[string]T) (Context, error) {
	m := make(map[string]Value, len(vars))
	for name, raw := range vars {
		v, err := FromJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("feel: variable %q: %w", name, err)
		}
		m[name] = v
	}
	return mapContext(m), nil
}
