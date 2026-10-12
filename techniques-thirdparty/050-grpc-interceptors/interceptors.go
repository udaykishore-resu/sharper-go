// Package grpcapi is Technique 50: interceptors for auth,
// logging and recovery, chained in a known order.
package grpcapi

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Verifier checks a signed token locally against cached keys.
type Verifier interface {
	Verify(token string) (principal string, err error)
}

// public is a deny-by-default allow-list of full method names.
var public = map[string]bool{
	"/grpc.health.v1.Health/Watch": true,
}

type principalKey struct{}

func authorize(ctx context.Context, v Verifier) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	vals := md.Get("authorization")
	if len(vals) != 1 {
		return nil, status.Error(codes.Unauthenticated, "missing token")
	}
	tok, ok := strings.CutPrefix(vals[0], "Bearer ")
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "bad token")
	}
	who, err := v.Verify(tok)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "bad token")
	}
	return context.WithValue(ctx, principalKey{}, who), nil
}

func Auth(v Verifier) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		info *grpc.UnaryServerInfo,
		next grpc.UnaryHandler) (any, error) {
		if public[info.FullMethod] {
			return next(ctx, req)
		}
		ctx, err := authorize(ctx, v)
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// Metrics counts calls by code; a stand-in for Technique 88.
type Metrics struct{ calls [17]atomic.Int64 }

func (m *Metrics) Count(c codes.Code) int64 { return m.calls[c].Load() }

// Log records the method, the code and the duration. It never
// logs the request message, which holds account IDs and amounts.
func Log(log *slog.Logger, m *Metrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		info *grpc.UnaryServerInfo,
		next grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := next(ctx, req)
		code := status.Code(err)
		m.calls[code].Add(1)
		log.InfoContext(ctx, "rpc", "method", info.FullMethod,
			"code", code.String(), "dur_ms", time.Since(start).Milliseconds())
		return resp, err
	}
}

// Recover matters more than in HTTP: grpc-go does not recover
// handler panics, so one panic would kill the server.
func Recover(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		info *grpc.UnaryServerInfo,
		next grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if v := recover(); v != nil {
				log.ErrorContext(ctx, "panic", "method", info.FullMethod, "panic", v)
				resp, err = nil, status.Error(codes.Internal, "internal error")
			}
		}()
		return next(ctx, req)
	}
}

// NewServer chains them; the first listed is outermost, so
// rejected calls still reach the log.
func NewServer(log *slog.Logger, m *Metrics, v Verifier) *grpc.Server {
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		Log(log, m), Recover(log), Auth(v)))
	return srv
}
