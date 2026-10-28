// Package handlers berisi handler job demo order-process. Handler sengaja idempoten: lease job
// bersifat at-least-once, jadi job yang sama bisa sampai ke handler lebih dari sekali.
package handlers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/indr622/workflow-engine/libs/go/sdk"
)

// Order adalah variabel instance demo.
type Order struct {
	OrderID string  `json:"orderId"`
	Amount  float64 `json:"amount"`
}

// ValidateOrder menolak order tanpa id atau dengan jumlah tidak positif lewat BPMN error.
func ValidateOrder(_ context.Context, job *sdk.Job) (sdk.Result, error) {
	var o Order
	if err := job.Variables.Unmarshal(&o); err != nil {
		return sdk.Result{}, err
	}
	if o.OrderID == "" || o.Amount <= 0 {
		return sdk.Result{}, sdk.BPMNError("INVALID_ORDER", fmt.Sprintf("order %q has amount %.2f", o.OrderID, o.Amount))
	}
	return sdk.Result{Variables: map[string]any{"validated": true}}, nil
}

// Payments mensimulasikan gateway pembayaran yang gagal sekali per instance, sehingga demo
// memperlihatkan FailJob dengan retry.
type Payments struct {
	seen    sync.Map // process instance key → struct{}
	Retries atomic.Int64
}

// Charge memenuhi sdk.Handler.
func (p *Payments) Charge(_ context.Context, job *sdk.Job) (sdk.Result, error) {
	if _, loaded := p.seen.LoadOrStore(job.ProcessInstanceKey, struct{}{}); !loaded {
		p.Retries.Add(1)
		return sdk.Result{}, sdk.Fail("payment gateway timeout", sdk.WithBackoff(0))
	}
	var o Order
	if err := job.Variables.Unmarshal(&o); err != nil {
		return sdk.Result{}, err
	}
	return sdk.Result{Variables: map[string]any{
		"paymentId": "pay-" + o.OrderID, "currency": job.CustomHeaders["currency"],
	}}, nil
}

// ShipOrder menandai order terkirim.
func ShipOrder(_ context.Context, job *sdk.Job) (sdk.Result, error) {
	id, err := job.Variables.String("orderId")
	if err != nil {
		return sdk.Result{}, err
	}
	return sdk.Result{Variables: map[string]any{"trackingNumber": "TRK-" + id}}, nil
}
