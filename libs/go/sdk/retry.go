package sdk

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"

	"connectrpc.com/connect"
)

// RetryPolicy mengulang panggilan yang gagal karena transport atau UNAVAILABLE. Hanya panggilan
// yang aman diulang yang dicoba lagi: query, ActivateJobs, PublishMessage dengan message_id, dan
// setiap mutasi yang membawa request_id (server menjalankannya sekali, ADR-0017).
type RetryPolicy struct {
	// MaxAttempts termasuk percobaan pertama; ≤ 1 mematikan retry.
	MaxAttempts int
	Backoff     BackoffPolicy
}

// DefaultRetryPolicy: 5 percobaan, 100 ms sampai 5 s dengan jitter.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 5, Backoff: BackoffPolicy{Initial: 100 * time.Millisecond, Max: 5 * time.Second, Multiplier: 2, Jitter: 0.2}}
}

// BackoffPolicy menghitung jeda eksponensial dengan jitter.
type BackoffPolicy struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
	// Jitter adalah fraksi acak ± dari jeda (0,2 = ±20 %).
	Jitter float64
}

// Delay mengembalikan jeda sebelum percobaan ke-attempt (mulai 1).
func (b BackoffPolicy) Delay(attempt int) time.Duration {
	if b.Initial <= 0 {
		return 0
	}
	mult := b.Multiplier
	if mult < 1 {
		mult = 1
	}
	d := float64(b.Initial)
	for i := 1; i < attempt; i++ {
		d *= mult
		if b.Max > 0 && d >= float64(b.Max) {
			d = float64(b.Max)
			break
		}
	}
	if b.Jitter > 0 {
		d += d * b.Jitter * (rand.Float64()*2 - 1) //nolint:gosec // jitter, bukan kriptografi
	}
	if b.Max > 0 && time.Duration(d) > b.Max {
		return b.Max
	}
	return time.Duration(d)
}

type requestIDCarrier interface{ GetRequestId() string }

type messageIDCarrier interface{ GetMessageId() string }

// retryable mengembalikan true bila request aman diulang.
func retryable(req connect.AnyRequest) bool {
	procedure := req.Spec().Procedure
	method := procedure[strings.LastIndex(procedure, "/")+1:]
	switch {
	case strings.HasPrefix(method, "Get"), strings.HasPrefix(method, "List"), method == "ActivateJobs", method == "ValidateResource":
		return true
	}
	if r, ok := req.Any().(requestIDCarrier); ok && r.GetRequestId() != "" {
		return true
	}
	if m, ok := req.Any().(messageIDCarrier); ok && m.GetMessageId() != "" {
		return true
	}
	return false
}

// transient: connect-go memetakan kegagalan dial/koneksi ke UNAVAILABLE, sama dengan engine yang
// sedang shutdown.
func transient(err error) bool { return connect.CodeOf(err) == connect.CodeUnavailable }

func retryInterceptor(p RetryPolicy) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if !retryable(req) {
				return next(ctx, req)
			}
			var (
				res connect.AnyResponse
				err error
			)
			for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
				res, err = next(ctx, req)
				if err == nil || !transient(err) || attempt == p.MaxAttempts {
					return res, err
				}
				t := time.NewTimer(p.Backoff.Delay(attempt))
				select {
				case <-ctx.Done():
					t.Stop()
					return nil, err
				case <-t.C:
				}
			}
			return res, err
		}
	}
}
