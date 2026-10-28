package sdk

import (
	"context"
	"time"

	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
)

// Job adalah job yang diaktifkan untuk worker.
type Job struct {
	Key                      string
	ProcessInstanceKey       string
	ElementInstanceKey       string
	ProcessDefinitionKey     string
	Type                     string
	ElementID                string
	BpmnProcessID            string
	Worker                   string
	ProcessDefinitionVersion int32
	Retries                  int32
	CustomHeaders            map[string]string
	Variables                Variables
	Deadline                 time.Time
}

func jobFromProto(j *jobv1.ActivatedJob) *Job {
	return &Job{
		Key: j.GetJobKey(), ProcessInstanceKey: j.GetProcessInstanceKey(), ElementInstanceKey: j.GetElementInstanceKey(),
		ProcessDefinitionKey: j.GetProcessDefinitionKey(), Type: j.GetType(), ElementID: j.GetElementId(),
		BpmnProcessID: j.GetBpmnProcessId(), Worker: j.GetWorker(), ProcessDefinitionVersion: j.GetProcessDefinitionVersion(),
		Retries: j.GetRetries(), CustomHeaders: j.GetCustomHeaders(), Variables: Variables(j.GetVariables()),
		Deadline: j.GetDeadline().AsTime(),
	}
}

// Result adalah hasil sukses handler; Variables nil berarti tanpa variabel.
type Result struct {
	Variables any
}

// Handler memproses satu job. Nilai kembalinya menentukan RPC yang dikirim JobWorker:
//
//	(Result, nil)            → CompleteJob(Result.Variables)
//	(_, *FailError)          → FailJob dengan retries, backoff, dan pesan dari FailError
//	(_, *BPMNErrorResult)    → ThrowError(code, message, variables)
//	(_, error lain)          → FailJob(job.Retries - 1, backoff kebijakan worker, err.Error())
//	ctx dibatalkan (shutdown) → tanpa RPC; job kembali lewat timeout lease di engine
type Handler func(ctx context.Context, job *Job) (Result, error)

// FailError meminta JobWorker menggagalkan job dengan parameter tertentu.
type FailError struct {
	Message string
	// Retries nil berarti job.Retries - 1.
	Retries *int32
	// Backoff nil berarti kebijakan FailureBackoff worker.
	Backoff   *time.Duration
	Variables any
}

func (e *FailError) Error() string { return "job failed: " + e.Message }

// FailOption mengatur Fail.
type FailOption func(*FailError)

// WithRetries menetapkan retries tersisa (0 = incident JOB_NO_RETRIES).
func WithRetries(n int32) FailOption { return func(e *FailError) { e.Retries = &n } }

// WithBackoff menetapkan jeda sebelum job dapat diaktifkan lagi.
func WithBackoff(d time.Duration) FailOption { return func(e *FailError) { e.Backoff = &d } }

// WithFailVariables menyertakan variabel pada FailJob.
func WithFailVariables(vars any) FailOption { return func(e *FailError) { e.Variables = vars } }

// Fail membuat error yang membuat JobWorker memanggil FailJob.
func Fail(message string, opts ...FailOption) error {
	e := &FailError{Message: message}
	for _, o := range opts {
		o(e)
	}
	return e
}

// BPMNErrorResult meminta JobWorker melempar BPMN error.
type BPMNErrorResult struct {
	Code      string
	Message   string
	Variables any
}

func (e *BPMNErrorResult) Error() string { return "bpmn error " + e.Code + ": " + e.Message }

// WithVariables menyertakan variabel pada ThrowError.
func (e *BPMNErrorResult) WithVariables(vars any) *BPMNErrorResult {
	e.Variables = vars
	return e
}

// BPMNError membuat error yang membuat JobWorker memanggil ThrowError dengan kode code.
func BPMNError(code, message string) *BPMNErrorResult {
	return &BPMNErrorResult{Code: code, Message: message}
}
