// Package keygen menghasilkan key unik global untuk semua entity engine (deployment, process
// definition, instance, element instance, job, variabel, incident) berupa UUIDv7 (RFC 9562).
//
// UUIDv7 dipilih (ADR-0011) agar key tidak bergantung pada sequence auto-increment database,
// bisa dibuat di proses engine mana pun tanpa koordinasi, dan tetap terurut menurut waktu
// sehingga indeks B-tree dan paginasi keyset tetap efisien. Di dalam satu proses, key yang
// dihasilkan selalu naik secara leksikografis (metode counter 12-bit RFC 9562 bagian 6.2).
package keygen

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"
)

// Generator menghasilkan key baru.
type Generator interface {
	// NextKeys mengembalikan n key berurutan naik.
	NextKeys(ctx context.Context, n int) ([]string, error)
}

// UUIDv7 adalah generator UUIDv7 monotonik yang aman dipakai banyak goroutine.
type UUIDv7 struct {
	mu     sync.Mutex
	now    func() time.Time
	random io.Reader
	lastMs int64
	seq    uint16 // counter 12-bit di rand_a
}

// New membuat generator di atas jam sistem dan crypto/rand.
func New() *UUIDv7 { return &UUIDv7{now: time.Now, random: rand.Reader} }

// NewWith membuat generator dengan jam dan sumber acak tertentu (untuk test).
func NewWith(now func() time.Time, random io.Reader) *UUIDv7 {
	return &UUIDv7{now: now, random: random}
}

var _ Generator = (*UUIDv7)(nil)

// NextKeys memenuhi Generator.
func (g *UUIDv7) NextKeys(_ context.Context, n int) ([]string, error) {
	out := make([]string, 0, max(n, 0))
	for i := 0; i < n; i++ {
		k, err := g.NewKey()
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// NewKey menghasilkan satu UUIDv7 dalam bentuk kanonik huruf kecil.
func (g *UUIDv7) NewKey() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(g.random, b[:]); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	g.mu.Lock()
	ms := g.now().UnixMilli()
	if ms > g.lastMs {
		g.lastMs = ms
		// Counter dimulai dari nilai acak dengan bit teratas 0 agar masih ada ruang naik.
		g.seq = (uint16(b[6])<<8 | uint16(b[7])) & 0x07ff
	} else {
		// Jam sama atau mundur: tetap pakai milidetik terakhir dan naikkan counter; bila counter
		// penuh, pinjam milidetik berikutnya agar urutan tetap naik.
		g.seq++
		if g.seq > 0x0fff {
			g.lastMs++
			g.seq = 0
		}
	}
	ms, seq := g.lastMs, g.seq
	g.mu.Unlock()

	// 48 bit milidetik big-endian; pemotongan ke byte memang disengaja.
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(ms)) //nolint:gosec // ms selalu positif (waktu Unix)
	copy(b[0:6], ts[2:8])
	var sq [2]byte
	binary.BigEndian.PutUint16(sq[:], seq)
	b[6] = 0x70 | sq[0]&0x0f
	b[7] = sq[1]
	b[8] = 0x80 | b[8]&0x3f
	return Format(b), nil
}

// Format mengubah 16 byte UUID ke bentuk kanonik 8-4-4-4-12 huruf kecil.
func Format(b [16]byte) string {
	var s [36]byte
	hex.Encode(s[0:8], b[0:4])
	s[8] = '-'
	hex.Encode(s[9:13], b[4:6])
	s[13] = '-'
	hex.Encode(s[14:18], b[6:8])
	s[18] = '-'
	hex.Encode(s[19:23], b[8:10])
	s[23] = '-'
	hex.Encode(s[24:], b[10:])
	return string(s[:])
}

// Valid mengembalikan true jika s adalah UUID kanonik (8-4-4-4-12, heksadesimal huruf kecil).
// Dipakai lapisan interfaces untuk menolak key yang rusak dengan INVALID_ARGUMENT.
func Valid(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return false
			}
		}
	}
	return true
}
