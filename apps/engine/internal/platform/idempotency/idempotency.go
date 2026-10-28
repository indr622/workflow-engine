// Package idempotency menyimpan klaim request_id RPC mutasi (ADR-0017). Klaim dilakukan di dalam
// transaksi command lewat INSERT ... ON CONFLICT DO NOTHING: duplikat paralel menunggu pada unique
// index sampai transaksi pertama selesai, lalu melihat response yang tersimpan, tanpa status
// "pending" yang terlihat dari luar.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

// MaxKeyBytes adalah panjang maksimum request_id.
const MaxKeyBytes = 128

// DefaultTTL adalah masa berlaku klaim bila tidak dikonfigurasi.
const DefaultTTL = 24 * time.Hour

// ErrKeyReused: request_id yang sama dipakai untuk request yang berbeda.
var ErrKeyReused = errors.New("idempotency key reused with a different request")

// ErrInvalidKey: request_id kosong, terlalu panjang, atau bukan UTF-8.
var ErrInvalidKey = errors.New("request_id must be 1 to 128 bytes of UTF-8")

// Claim adalah satu klaim request_id.
type Claim struct {
	// Scope memisahkan RPC: tenant + "/" + nama RPC lengkap.
	Scope       string
	Key         string
	RequestHash [32]byte
}

// ValidateKey memeriksa bentuk request_id.
func ValidateKey(key string) error {
	if key == "" || len(key) > MaxKeyBytes || !utf8.ValidString(key) {
		return ErrInvalidKey
	}
	return nil
}

// Hash menghasilkan SHA-256 dari marshal proto deterministik setelah request_id dikosongkan oleh
// clear (yang menerima salinan pesan).
func Hash(msg proto.Message, clear func(proto.Message)) ([32]byte, error) {
	c := proto.Clone(msg)
	if clear != nil {
		clear(c)
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(c)
	if err != nil {
		return [32]byte{}, fmt.Errorf("hash request: %w", err)
	}
	return sha256.Sum256(data), nil
}

// Outcome adalah hasil Begin.
type Outcome uint8

const (
	// Claimed: jalankan command lalu simpan response sebelum commit.
	Claimed Outcome = iota + 1
	// Replayed: response tersimpan dikembalikan; jangan jalankan command.
	Replayed
)

// Result adalah hasil Begin.
type Result struct {
	Outcome  Outcome
	Response []byte
}

// Begin mengklaim request_id di dalam tx. Harus dipanggil SETELAH lock aggregate agar urutan xid
// per aggregate tetap sama dengan urutan lock (ADR-0015).
func Begin(ctx context.Context, tx pgx.Tx, c Claim, ttl time.Duration) (Result, error) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	for attempt := 0; attempt < 2; attempt++ {
		tag, err := tx.Exec(ctx, `INSERT INTO idempotency_key (scope, key, request_hash, expires_at)
VALUES ($1, $2, $3, now() + make_interval(secs => $4)) ON CONFLICT (scope, key) DO NOTHING`,
			c.Scope, c.Key, c.RequestHash[:], ttl.Seconds())
		if err != nil {
			return Result{}, fmt.Errorf("claim request_id: %w", err)
		}
		if tag.RowsAffected() == 1 {
			return Result{Outcome: Claimed}, nil
		}
		var (
			hash     []byte
			response []byte
			expired  bool
		)
		err = tx.QueryRow(ctx, `SELECT request_hash, response, expires_at < now() FROM idempotency_key WHERE scope = $1 AND key = $2`,
			c.Scope, c.Key).Scan(&hash, &response, &expired)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // baris dihapus janitor di antara dua statement; klaim ulang
		}
		if err != nil {
			return Result{}, fmt.Errorf("read request_id claim: %w", err)
		}
		if expired {
			if _, err := tx.Exec(ctx, `DELETE FROM idempotency_key WHERE scope = $1 AND key = $2`, c.Scope, c.Key); err != nil {
				return Result{}, err
			}
			continue
		}
		if !bytes.Equal(hash, c.RequestHash[:]) {
			return Result{}, ErrKeyReused
		}
		if response == nil {
			return Result{}, fmt.Errorf("request_id claim of %s has no stored response", c.Key)
		}
		return Result{Outcome: Replayed, Response: response}, nil
	}
	return Result{}, fmt.Errorf("could not claim request_id %q", c.Key)
}

// StoreSQL mengisi response pada baris yang diklaim transaksi ini; dipakai lewat pgx.Batch.
const StoreSQL = `UPDATE idempotency_key SET response = $3 WHERE scope = $1 AND key = $2`

// QueueStore mengantrekan penyimpanan response ke batch command.
func QueueStore(b *pgx.Batch, c Claim, response []byte) {
	if response == nil {
		response = []byte{}
	}
	b.Queue(StoreSQL, c.Scope, c.Key, response)
}

// Prune menghapus klaim kedaluwarsa dalam batch.
func Prune(ctx context.Context, pool *pgxpool.Pool, limit int) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM idempotency_key WHERE ctid IN (
SELECT ctid FROM idempotency_key WHERE expires_at < now() LIMIT $1)`, limit)
	if err != nil {
		return 0, fmt.Errorf("prune idempotency keys: %w", err)
	}
	return tag.RowsAffected(), nil
}
