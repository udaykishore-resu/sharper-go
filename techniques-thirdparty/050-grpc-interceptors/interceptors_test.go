package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type tokens map[string]string

func (t tokens) Verify(tok string) (string, error) {
	if who, ok := t[tok]; ok {
		return who, nil
	}
	return "", errors.New("unknown token")
}

func dial(t *testing.T, m *Metrics) healthpb.HealthClient {
	t.Helper()
	ln := bufconn.Listen(1 << 20)
	srv := NewServer(slog.New(slog.DiscardHandler), m, tokens{"ok": "acme"})
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go srv.Serve(ln)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ln.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return healthpb.NewHealthClient(conn)
}

// A method nobody listed is protected the moment it exists.
func TestDenyByDefault(t *testing.T) {
	m := &Metrics{}
	c := dial(t, m)
	_, err := c.Check(t.Context(), &healthpb.HealthCheckRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no token: %v", err)
	}
	ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer ok")
	if _, err := c.Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("with token: %v", err)
	}
	if m.Count(codes.Unauthenticated) != 1 || m.Count(codes.OK) != 1 {
		t.Fatal("Log runs outermost, so rejections are counted too")
	}
}

func TestRecover(t *testing.T) {
	_, err := Recover(slog.New(slog.DiscardHandler))(t.Context(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/ledger.v1.Ledger/ReverseTransfer"},
		func(context.Context, any) (any, error) { panic("nil map") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("err = %v", err)
	}
}
