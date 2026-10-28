package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Variables adalah dokumen JSON object variabel job atau instance.
type Variables json.RawMessage

// Unmarshal men-decode variabel ke dst. Angka di-decode dengan UseNumber bila dst adalah
// map[string]any atau any, sehingga bilangan bulat besar tidak kehilangan presisi.
func (v Variables) Unmarshal(dst any) error {
	if len(v) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()
	return dec.Decode(dst)
}

// Get mengembalikan nilai JSON satu variabel.
func (v Variables) Get(name string) (json.RawMessage, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(v, &m); err != nil {
		return nil, false
	}
	raw, ok := m[name]
	return raw, ok
}

// String mengembalikan variabel string.
func (v Variables) String(name string) (string, error) {
	var s string
	return s, v.decodeOne(name, &s)
}

// Int64 mengembalikan variabel bilangan bulat.
func (v Variables) Int64(name string) (int64, error) {
	var n json.Number
	if err := v.decodeOne(name, &n); err != nil {
		return 0, err
	}
	return n.Int64()
}

// Float64 mengembalikan variabel bilangan.
func (v Variables) Float64(name string) (float64, error) {
	var f float64
	return f, v.decodeOne(name, &f)
}

// Bool mengembalikan variabel boolean.
func (v Variables) Bool(name string) (bool, error) {
	var b bool
	return b, v.decodeOne(name, &b)
}

func (v Variables) decodeOne(name string, dst any) error {
	raw, ok := v.Get(name)
	if !ok {
		return fmt.Errorf("sdk: variable %q not found", name)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("sdk: variable %q: %w", name, err)
	}
	return nil
}

// MarshalVariables mengubah nilai variabel menjadi dokumen JSON object. Menerima nil (kosong),
// map, struct, json.RawMessage, Variables, []byte, atau string berisi JSON object.
func MarshalVariables(v any) (string, error) {
	var raw []byte
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		raw = []byte(x)
	case []byte:
		raw = x
	case json.RawMessage:
		raw = x
	case Variables:
		raw = x
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidVariables, err)
		}
		raw = b
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", nil
	}
	if trimmed[0] != '{' || !json.Valid(trimmed) {
		return "", ErrInvalidVariables
	}
	return string(trimmed), nil
}
