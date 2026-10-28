package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxVariableNameBytes adalah panjang maksimum nama variabel.
const MaxVariableNameBytes = 256

// NamedValue adalah satu variabel masukan dengan nilai JSON ringkas.
type NamedValue struct {
	Name  string
	Value json.RawMessage
}

// ParseVariables mem-parse dokumen JSON object menjadi daftar variabel top-level, mempertahankan
// urutan kemunculan pertama (key ganda: nilai terakhir menang, seperti encoding/json). Dokumen
// kosong atau hanya spasi berarti tanpa variabel. Angka tidak dikonversi sehingga presisi utuh.
func ParseVariables(raw []byte, maxBytes int) ([]NamedValue, error) {
	if maxBytes > 0 && len(raw) > maxBytes {
		return nil, fmt.Errorf("%w: document is %d bytes, limit is %d", ErrInvalidVariables, len(raw), maxBytes)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVariables, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("%w: document must be a JSON object", ErrInvalidVariables)
	}
	var out []NamedValue
	index := map[string]int{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidVariables, err)
		}
		name, _ := tok.(string)
		if name == "" {
			return nil, fmt.Errorf("%w: variable name must not be empty", ErrInvalidVariables)
		}
		if len(name) > MaxVariableNameBytes {
			return nil, fmt.Errorf("%w: variable name exceeds %d bytes", ErrInvalidVariables, MaxVariableNameBytes)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("%w: variable %q: %v", ErrInvalidVariables, name, err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, value); err != nil {
			return nil, fmt.Errorf("%w: variable %q: %v", ErrInvalidVariables, name, err)
		}
		if i, ok := index[name]; ok {
			out[i].Value = compact.Bytes()
			continue
		}
		index[name] = len(out)
		out = append(out, NamedValue{Name: name, Value: compact.Bytes()})
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVariables, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: unexpected data after the JSON object", ErrInvalidVariables)
	}
	return out, nil
}
