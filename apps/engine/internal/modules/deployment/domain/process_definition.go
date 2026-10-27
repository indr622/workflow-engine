// Package domain berisi aturan bisnis deployment: versi process definition dan dedup checksum.
// Paket ini tidak mengenal format BPMN; parsing dilakukan di lapisan application.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// DefaultTenant adalah tenant tunggal sampai multi-tenant diaktifkan (Fase 8).
const DefaultTenant = "<default>"

// Checksum adalah SHA-256 atas byte resource mentah.
type Checksum [sha256.Size]byte

// ChecksumOf menghitung checksum konten.
func ChecksumOf(content []byte) Checksum { return sha256.Sum256(content) }

// ChecksumFromBytes membaca checksum dari penyimpanan.
func ChecksumFromBytes(b []byte) (Checksum, error) {
	var c Checksum
	if len(b) != sha256.Size {
		return c, fmt.Errorf("checksum must be %d bytes, got %d", sha256.Size, len(b))
	}
	copy(c[:], b)
	return c, nil
}

// Bytes mengembalikan checksum sebagai slice untuk penyimpanan.
func (c Checksum) Bytes() []byte { return c[:] }

func (c Checksum) String() string { return hex.EncodeToString(c[:]) }

// ProcessDefinition adalah satu versi proses yang ter-deploy.
type ProcessDefinition struct {
	Key           int64
	TenantID      string
	BpmnProcessID string
	Version       int32
	VersionTag    string
	Name          string
	ResourceName  string
	Checksum      Checksum
	DeploymentKey int64
	CreatedAt     time.Time
}

// NextVersion menentukan versi berikutnya: 1 bila belum ada, selain itu naik satu.
func NextVersion(latest *ProcessDefinition) int32 {
	if latest == nil {
		return 1
	}
	return latest.Version + 1
}

// IsDuplicateOf mengembalikan true bila definition ini berasal dari resource dengan checksum sama.
// Dedup hanya dibandingkan terhadap versi terbaru (keputusan 9.1 #5 rencana Fase 1).
func (pd ProcessDefinition) IsDuplicateOf(c Checksum) bool { return pd.Checksum == c }
