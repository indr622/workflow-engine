// Package domain berisi aturan buffer message (ADR-0022): validasi, TTL, dan dedup message_id.
package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Error adalah error aturan domain dengan reason stabil (ErrorInfo.reason, ADR-0023).
type Error struct {
	reason, msg string
	metadata    map[string]string
}

func (e *Error) Error() string { return e.msg }

// Reason mengembalikan kode stabil.
func (e *Error) Reason() string { return e.reason }

// ErrorMetadata mengembalikan metadata ErrorInfo.
func (e *Error) ErrorMetadata() map[string]string { return e.metadata }

// Is membuat errors.Is cocok berdasarkan reason, sehingga error dengan metadata tetap sama dengan
// sentinel-nya.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.reason == e.reason
}

// Error domain.
var (
	ErrDuplicate        = &Error{reason: "MESSAGE_DUPLICATE", msg: "a message with this message_id is still buffered"}
	ErrInvalidMessage   = &Error{reason: "REQUEST_INVALID", msg: "invalid message"}
	ErrInvalidVariables = &Error{reason: "VARIABLES_INVALID", msg: "invalid variables"}
)

// DuplicateOf mengembalikan ErrDuplicate dengan key message yang sudah ada di metadata.
func DuplicateOf(existingKey string) error {
	return &Error{reason: ErrDuplicate.reason, msg: ErrDuplicate.msg, metadata: map[string]string{"message_key": existingKey}}
}

// Message adalah satu message di buffer.
type Message struct {
	Key            string
	TenantID       string
	Name           string
	CorrelationKey string
	MessageID      string
	TTL            time.Duration
	// Variables adalah JSON object ringkas.
	Variables []byte
}

// Policy membatasi message yang diterima.
type Policy struct {
	MaxTTL            time.Duration
	MaxVariablesBytes int
}

// Validate memeriksa message dan meringkas dokumen variabelnya.
func (p Policy) Validate(m *Message) error {
	if m.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidMessage)
	}
	if m.TTL < 0 {
		return fmt.Errorf("%w: time_to_live must not be negative", ErrInvalidMessage)
	}
	if p.MaxTTL > 0 && m.TTL > p.MaxTTL {
		return fmt.Errorf("%w: time_to_live %s exceeds the maximum %s", ErrInvalidMessage, m.TTL, p.MaxTTL)
	}
	vars, err := compactObject(m.Variables, p.MaxVariablesBytes)
	if err != nil {
		return err
	}
	m.Variables = vars
	return nil
}

// Buffered mengembalikan true bila message disimpan (TTL > 0); TTL 0 diterima lalu dibuang.
func (m *Message) Buffered() bool { return m.TTL > 0 }

func compactObject(raw []byte, maxBytes int) ([]byte, error) {
	if maxBytes > 0 && len(raw) > maxBytes {
		return nil, fmt.Errorf("%w: document is %d bytes, limit is %d", ErrInvalidVariables, len(raw), maxBytes)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []byte("{}"), nil
	}
	if trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, fmt.Errorf("%w: variables must be a JSON object", ErrInvalidVariables)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVariables, err)
	}
	return buf.Bytes(), nil
}

// Repository menyimpan message.
type Repository interface {
	// Insert menyimpan message dengan expires_at = sekarang + TTL (jam Postgres). Bila message_id
	// yang sama masih di buffer, kembalikan DuplicateOf(key yang ada).
	Insert(ctx context.Context, m Message) (expiresAt time.Time, err error)
	// DeleteExpired menghapus sampai batch message kedaluwarsa (SKIP LOCKED).
	DeleteExpired(ctx context.Context, batch int) (int, error)
}
