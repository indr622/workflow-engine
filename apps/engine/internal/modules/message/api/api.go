// Package api adalah kontrak publik modul message: publikasi message ke buffer dan event yang
// dipakai korelasi Fase 5.
package api

import (
	"context"
	"time"
)

// PublishRequest adalah message yang dipublikasikan klien.
type PublishRequest struct {
	Name           string
	CorrelationKey string
	// TTL 0 berarti message diterima tetapi tidak disimpan.
	TTL       time.Duration
	MessageID string
	// Variables adalah dokumen JSON object; kosong berarti tanpa variabel.
	Variables []byte
}

// Messages mempublikasikan message.
type Messages interface {
	// Publish mengembalikan key message baru.
	Publish(ctx context.Context, req PublishRequest) (messageKey string, err error)
}

// TopicMessagePublished dipublikasikan setelah message tersimpan; Fase 5 berlangganan untuk
// korelasi ke subscription.
const TopicMessagePublished = "message.published"

// MessagePublished adalah event message yang baru dipublikasikan.
type MessagePublished struct {
	MessageKey     string
	Name           string
	CorrelationKey string
	ExpiresAt      time.Time
}

// Topic memenuhi eventbus.Event.
func (MessagePublished) Topic() string { return TopicMessagePublished }
