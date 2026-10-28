// Package domain berisi aturan murni protokol job worker: batas server dan klem parameter
// ActivateJobs, UpdateJobTimeout, dan FailJob. Nilai di luar batas dipotong, bukan ditolak, agar
// worker yang memakai default besar tidak gagal setelah batas server diturunkan (bagian 4.1).
package domain

import "time"

// Limits adalah batas server protokol worker.
type Limits struct {
	DefaultMaxJobs    int32
	MaxJobs           int32
	DefaultTimeout    time.Duration
	MinTimeout        time.Duration
	MaxTimeout        time.Duration
	MaxRequestTimeout time.Duration
	MaxBackoff        time.Duration
	// PollInterval adalah polling cadangan long polling untuk job yang tidak memicu notifikasi
	// (replika lain, backoff yang berakhir, resolve incident).
	PollInterval time.Duration
	// MaxLongPolls membatasi long polling aktif; di atasnya request dijawab dengan satu kueri.
	MaxLongPolls int
	// DefaultWorker dipakai bila request tidak memberi nama worker.
	DefaultWorker string
}

// DefaultLimits mengembalikan batas bawaan (rencana Fase 4, bagian 4.1).
func DefaultLimits() Limits {
	return Limits{
		DefaultMaxJobs: 32, MaxJobs: 1000,
		DefaultTimeout: 5 * time.Minute, MinTimeout: time.Second, MaxTimeout: 24 * time.Hour,
		MaxRequestTimeout: 55 * time.Second, MaxBackoff: 24 * time.Hour,
		PollInterval: time.Second, MaxLongPolls: 1024, DefaultWorker: "default",
	}
}

// WithDefaults mengisi nilai nol dengan DefaultLimits.
func (l Limits) WithDefaults() Limits {
	d := DefaultLimits()
	if l.DefaultMaxJobs <= 0 {
		l.DefaultMaxJobs = d.DefaultMaxJobs
	}
	if l.MaxJobs <= 0 {
		l.MaxJobs = d.MaxJobs
	}
	if l.DefaultTimeout <= 0 {
		l.DefaultTimeout = d.DefaultTimeout
	}
	if l.MinTimeout <= 0 {
		l.MinTimeout = d.MinTimeout
	}
	if l.MaxTimeout <= 0 {
		l.MaxTimeout = d.MaxTimeout
	}
	if l.MaxRequestTimeout <= 0 {
		l.MaxRequestTimeout = d.MaxRequestTimeout
	}
	if l.MaxBackoff <= 0 {
		l.MaxBackoff = d.MaxBackoff
	}
	if l.PollInterval <= 0 {
		l.PollInterval = d.PollInterval
	}
	if l.MaxLongPolls <= 0 {
		l.MaxLongPolls = d.MaxLongPolls
	}
	if l.DefaultWorker == "" {
		l.DefaultWorker = d.DefaultWorker
	}
	return l
}

// ActivationRequest adalah parameter ActivateJobs dari worker.
type ActivationRequest struct {
	Type           string
	Worker         string
	MaxJobs        int32
	Timeout        time.Duration
	FetchVariables []string
	// RequestTimeout 0 berarti jawab segera (satu kueri).
	RequestTimeout time.Duration
}

// Clamp mengisi default dan memotong setiap parameter ke batas.
func (r ActivationRequest) Clamp(l Limits) ActivationRequest {
	if r.Worker == "" {
		r.Worker = l.DefaultWorker
	}
	switch {
	case r.MaxJobs <= 0:
		r.MaxJobs = l.DefaultMaxJobs
	case r.MaxJobs > l.MaxJobs:
		r.MaxJobs = l.MaxJobs
	}
	if r.Timeout <= 0 {
		r.Timeout = l.DefaultTimeout
	}
	r.Timeout = l.ClampTimeout(r.Timeout)
	r.RequestTimeout = min(max(r.RequestTimeout, 0), l.MaxRequestTimeout)
	if len(r.FetchVariables) == 0 {
		r.FetchVariables = nil
	}
	return r
}

// ClampTimeout memotong durasi lease ke [MinTimeout, MaxTimeout].
func (l Limits) ClampTimeout(d time.Duration) time.Duration {
	return min(max(d, l.MinTimeout), l.MaxTimeout)
}

// ClampBackoff memotong backoff FailJob ke [0, MaxBackoff].
func (l Limits) ClampBackoff(d time.Duration) time.Duration {
	return min(max(d, 0), l.MaxBackoff)
}
