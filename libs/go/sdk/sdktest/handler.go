package sdktest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/libs/go/sdk"
)

// Outcome adalah hasil handler yang akan dikirim JobWorker ke engine.
type Outcome struct {
	Completed bool
	Variables any
	Failed    bool
	// Retries dan Backoff hanya diisi bila handler mengembalikan sdk.Fail dengan opsi tersebut.
	Retries      *int32
	Backoff      *time.Duration
	ErrorMessage string
	ErrorCode    string
	Err          error
}

// RunHandler memanggil handler secara langsung untuk unit test aplikasi pengguna.
func RunHandler(t testing.TB, h sdk.Handler, job *sdk.Job) Outcome {
	t.Helper()
	if job.Variables == nil {
		job.Variables = sdk.Variables("{}")
	}
	res, err := h(context.Background(), job)
	var fail *sdk.FailError
	var bpmn *sdk.BPMNErrorResult
	switch {
	case err == nil:
		return Outcome{Completed: true, Variables: res.Variables}
	case errors.As(err, &bpmn):
		return Outcome{ErrorCode: bpmn.Code, ErrorMessage: bpmn.Message, Variables: bpmn.Variables, Err: err}
	case errors.As(err, &fail):
		return Outcome{Failed: true, Retries: fail.Retries, Backoff: fail.Backoff, ErrorMessage: fail.Message, Variables: fail.Variables, Err: err}
	default:
		return Outcome{Failed: true, ErrorMessage: err.Error(), Err: err}
	}
}
