package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Batas bawaan (bagian 8.6 rencana Fase 4).
const (
	// MaxRequestBytes adalah ukuran request maksimum semua handler; dokumen variabel dibatasi
	// terpisah oleh ENGINE_MAX_VARIABLES_BYTES.
	MaxRequestBytes = 16 << 20
	// DefaultTimeout membatasi RPC biasa.
	DefaultTimeout = 30 * time.Second
	// MaxTimeout membatasi RPC panjang (long polling, create with result).
	MaxTimeout = 60 * time.Second
	// longPollGrace ditambahkan ke request_timeout agar handler sempat menjawab kosong.
	longPollGrace = 5 * time.Second
)

// Config mengatur rantai interceptor.
type Config struct {
	Log            *slog.Logger
	DefaultTimeout time.Duration
	// Authenticator menentukan principal; nil berarti Anonymous (Fase 4).
	Authenticator Authenticator
}

// HandlerOptions mengembalikan opsi yang dipasang ke SETIAP handler Connect: recover, batas ukuran,
// lalu interceptor (luar → dalam) logging, timeout, authn, validate.
func HandlerOptions(cfg Config) ([]connect.HandlerOption, error) {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = DefaultTimeout
	}
	if cfg.Authenticator == nil {
		cfg.Authenticator = Anonymous{}
	}
	validator, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("protovalidate: %w", err)
	}
	return []connect.HandlerOption{
		connect.WithRecover(recoverPanic(cfg.Log)),
		connect.WithReadMaxBytes(MaxRequestBytes),
		connect.WithInterceptors(
			logging(cfg.Log),
			timeout(cfg.DefaultTimeout),
			authn(cfg.Authenticator),
			validate(validator),
		),
	}, nil
}

func recoverPanic(log *slog.Logger) func(context.Context, connect.Spec, http.Header, any) error {
	return func(ctx context.Context, spec connect.Spec, _ http.Header, p any) error {
		log.ErrorContext(ctx, "rpc panic", slog.String("procedure", spec.Procedure), slog.Any("panic", p),
			slog.String("stack", string(debug.Stack())))
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}

// logging menulis satu baris per RPC. Sukses di level debug agar long polling tidak membanjiri
// log; error klien di level info; error internal di level error.
func logging(log *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			res, err := next(ctx, req)
			attrs := []slog.Attr{
				slog.String("procedure", req.Spec().Procedure),
				slog.String("code", codeOf(err)),
				slog.Duration("duration", time.Since(start)),
				slog.String("principal", FromContext(ctx).Subject),
			}
			if id := req.Header().Get("X-Request-Id"); id != "" {
				attrs = append(attrs, slog.String("request_id", id))
			}
			level := slog.LevelDebug
			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
				level = slog.LevelInfo
				if connect.CodeOf(err) == connect.CodeInternal || connect.CodeOf(err) == connect.CodeUnknown {
					level = slog.LevelError
				}
			}
			log.LogAttrs(ctx, level, "rpc", attrs...)
			return res, err
		}
	}
}

func codeOf(err error) string {
	if err == nil {
		return "ok"
	}
	return connect.CodeOf(err).String()
}

// requestTimeouter dipenuhi request RPC panjang (ActivateJobs, CreateProcessInstanceWithResult).
type requestTimeouter interface {
	GetRequestTimeout() *durationpb.Duration
}

// timeout memasang batas waktu per RPC; RPC panjang mendapat request_timeout + 5 s (maks 60 s).
// Batas klien (header Connect-Timeout-Ms) tetap berlaku bila lebih kecil karena context-nya sudah
// dipasang connect-go.
func timeout(def time.Duration) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			d := def
			if rt, ok := req.Any().(requestTimeouter); ok && rt.GetRequestTimeout() != nil {
				d = min(rt.GetRequestTimeout().AsDuration()+longPollGrace, MaxTimeout)
			}
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, req)
		}
	}
}

// validate menolak request yang melanggar aturan buf.validate di proto.
func validate(v protovalidate.Validator) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			msg, ok := req.Any().(proto.Message)
			if !ok {
				return next(ctx, req)
			}
			err := v.Validate(msg)
			if err == nil {
				return next(ctx, req)
			}
			var verr *protovalidate.ValidationError
			if !errors.As(err, &verr) {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("validate request: %w", err))
			}
			parts := make([]string, 0, len(verr.Violations))
			for _, vi := range verr.Violations {
				parts = append(parts, vi.String())
			}
			cerr := Reject(connect.CodeInvalidArgument, ReasonRequestInvalid, errors.New(strings.Join(parts, "; ")),
				map[string]string{"violations": strings.Join(parts, "; ")})
			if detail, derr := connect.NewErrorDetail(verr.ToProto()); derr == nil {
				cerr.AddDetail(detail)
			}
			return nil, cerr
		}
	}
}
