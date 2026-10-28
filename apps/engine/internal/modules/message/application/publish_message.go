// Package application berisi use case modul message: publikasi ke buffer dan sweeper TTL.
package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/message/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/message/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/metrics"
)

// KeyGenerator memberi key UUIDv7 baru.
type KeyGenerator interface {
	NewKey() (string, error)
}

// PublishMessage mengimplementasikan api.Messages.
type PublishMessage struct {
	repo   domain.Repository
	keys   KeyGenerator
	bus    eventbus.Bus
	policy domain.Policy
	log    *slog.Logger
}

var _ api.Messages = (*PublishMessage)(nil)

// DefaultTenant adalah tenant tunggal sampai Fase 8.
const DefaultTenant = "<default>"

// NewPublishMessage merakit use case.
func NewPublishMessage(repo domain.Repository, keys KeyGenerator, bus eventbus.Bus, policy domain.Policy, log *slog.Logger) *PublishMessage {
	if log == nil {
		log = slog.Default()
	}
	return &PublishMessage{repo: repo, keys: keys, bus: bus, policy: policy, log: log}
}

// Publish memenuhi api.Messages. Message dengan TTL 0 mendapat key tetapi tidak disimpan, sama
// dengan Zeebe untuk message tanpa subscription yang menunggu.
func (p *PublishMessage) Publish(ctx context.Context, req api.PublishRequest) (string, error) {
	m := domain.Message{
		TenantID: DefaultTenant, Name: req.Name, CorrelationKey: req.CorrelationKey, MessageID: req.MessageID,
		TTL: req.TTL, Variables: req.Variables,
	}
	if err := p.policy.Validate(&m); err != nil {
		return "", err
	}
	key, err := p.keys.NewKey()
	if err != nil {
		return "", fmt.Errorf("message key: %w", err)
	}
	m.Key = key
	if !m.Buffered() {
		metrics.Get().MessagesPublished.Add(ctx, 1, metrics.Attr("outcome", "not_buffered"))
		return key, nil
	}
	expiresAt, err := p.repo.Insert(ctx, m)
	if err != nil {
		metrics.Get().MessagesPublished.Add(ctx, 1, metrics.Attr("outcome", "rejected"))
		return "", err
	}
	metrics.Get().MessagesPublished.Add(ctx, 1, metrics.Attr("outcome", "buffered"))
	if p.bus != nil {
		ev := api.MessagePublished{MessageKey: key, Name: m.Name, CorrelationKey: m.CorrelationKey, ExpiresAt: expiresAt}
		if err := p.bus.Publish(ctx, ev); err != nil {
			p.log.Warn("publish message event", slog.String("message_key", key), slog.Any("error", err))
		}
	}
	return key, nil
}

// SweepExpired menghapus message kedaluwarsa. Memenuhi sweeper.Task.
type SweepExpired struct {
	repo domain.Repository
}

// NewSweepExpired merakit task.
func NewSweepExpired(repo domain.Repository) *SweepExpired { return &SweepExpired{repo: repo} }

// Name memenuhi sweeper.Task.
func (s *SweepExpired) Name() string { return "message_ttl" }

// Sweep memenuhi sweeper.Task.
func (s *SweepExpired) Sweep(ctx context.Context, batch int) (int, error) {
	return s.repo.DeleteExpired(ctx, batch)
}
