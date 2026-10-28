package rpc

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
)

// DefaultTenant adalah tenant tunggal sampai multi-tenant Fase 8.
const DefaultTenant = "<default>"

// Principal adalah identitas pemanggil RPC (ADR-0023). Fase 4 selalu anonim; Fase 8 mengisinya
// dari token OIDC tanpa mengubah handler.
type Principal struct {
	Subject string
	Tenant  string
}

// AnonymousPrincipal dipakai bila request tidak terautentikasi.
var AnonymousPrincipal = Principal{Subject: "anonymous", Tenant: DefaultTenant}

// Authenticator menentukan principal dari header request.
type Authenticator interface {
	Authenticate(ctx context.Context, header http.Header) (Principal, error)
}

// Anonymous adalah Authenticator Fase 4: header Authorization diterima apa adanya (tidak
// diverifikasi) dan setiap request menjadi anonim. Tidak ada request yang ditolak.
type Anonymous struct{}

// Authenticate memenuhi Authenticator.
func (Anonymous) Authenticate(context.Context, http.Header) (Principal, error) {
	return AnonymousPrincipal, nil
}

type principalKey struct{}

// WithPrincipal memasang principal ke context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext mengembalikan principal request, atau AnonymousPrincipal bila tidak ada.
func FromContext(ctx context.Context) Principal {
	if p, ok := ctx.Value(principalKey{}).(Principal); ok {
		return p
	}
	return AnonymousPrincipal
}

func authn(a Authenticator) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			p, err := a.Authenticate(ctx, req.Header())
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}
			return next(WithPrincipal(ctx, p), req)
		}
	}
}
