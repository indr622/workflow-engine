// Package paging membuat token halaman keyset yang opak bagi klien: base64url dari JSON berisi
// posisi terakhir. Klien tidak boleh menyusun token sendiri; token salah bentuk ditolak.
package paging

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// ErrInvalidToken dikembalikan untuk token yang tidak bisa didekode.
var ErrInvalidToken = errors.New("invalid page token")

// Token adalah posisi terakhir halaman sebelumnya: key (string) atau sequence (angka).
type Token struct {
	Key      string `json:"k,omitempty"`
	Sequence int32  `json:"s,omitempty"`
}

// Encode menghasilkan token opak; posisi kosong menghasilkan string kosong (tidak ada halaman berikutnya).
func Encode(t Token) string {
	if t.Key == "" && t.Sequence == 0 {
		return ""
	}
	data, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// Decode membaca token; string kosong berarti halaman pertama.
func Decode(s string) (Token, error) {
	if s == "" {
		return Token{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Token{}, ErrInvalidToken
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil || t.Sequence < 0 {
		return Token{}, ErrInvalidToken
	}
	return t, nil
}
