package handlers_test

import (
	"testing"

	"github.com/indr622/workflow-engine/apps/worker-example/internal/handlers"
	"github.com/indr622/workflow-engine/libs/go/sdk"
	"github.com/indr622/workflow-engine/libs/go/sdk/sdktest"
)

func TestValidateOrder(t *testing.T) {
	ok := sdktest.RunHandler(t, handlers.ValidateOrder, &sdk.Job{Variables: sdk.Variables(`{"orderId":"A1","amount":10}`)})
	if !ok.Completed {
		t.Fatalf("valid order = %+v", ok)
	}
	bad := sdktest.RunHandler(t, handlers.ValidateOrder, &sdk.Job{Variables: sdk.Variables(`{"orderId":"","amount":10}`)})
	if bad.ErrorCode != "INVALID_ORDER" {
		t.Fatalf("invalid order = %+v", bad)
	}
}

func TestChargeFailsOncePerInstance(t *testing.T) {
	p := &handlers.Payments{}
	job := &sdk.Job{ProcessInstanceKey: "pi-1", Variables: sdk.Variables(`{"orderId":"A1","amount":10}`),
		CustomHeaders: map[string]string{"currency": "IDR"}}
	first := sdktest.RunHandler(t, p.Charge, job)
	if !first.Failed || first.Backoff == nil || *first.Backoff != 0 {
		t.Fatalf("first charge = %+v", first)
	}
	second := sdktest.RunHandler(t, p.Charge, job)
	if !second.Completed || second.Variables.(map[string]any)["currency"] != "IDR" {
		t.Fatalf("second charge = %+v", second)
	}
	if p.Retries.Load() != 1 {
		t.Fatalf("retries = %d", p.Retries.Load())
	}
}

func TestShipOrder(t *testing.T) {
	out := sdktest.RunHandler(t, handlers.ShipOrder, &sdk.Job{Variables: sdk.Variables(`{"orderId":"A1"}`)})
	if !out.Completed || out.Variables.(map[string]any)["trackingNumber"] != "TRK-A1" {
		t.Fatalf("ship = %+v", out)
	}
	if out := sdktest.RunHandler(t, handlers.ShipOrder, &sdk.Job{}); !out.Failed {
		t.Fatalf("missing order id = %+v", out)
	}
}
