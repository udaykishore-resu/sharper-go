//go:build integration

package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"example.com/ledgerd/internal/store"
)

// Technique 26's check: the fake and the real store must agree.
// The same requests go through the production chain twice, once
// over fakeStore and once over Postgres, and must get the same
// status and the same stable code. The detail is English and may
// differ (the real store adds "transfer <id>:"), which is why
// clients switch on the code.
func TestFakeAgreesWithRealStore(t *testing.T) {
	dsn := os.Getenv("LEDGERD_TEST_DATABASE_URL")
	if dsn == "" {
		// Technique 7: a tagged run with no database fails; a skip
		// would let CI report green without running anything.
		t.Fatal("integration tag set but LEDGERD_TEST_DATABASE_URL is empty")
	}
	ctx := t.Context()
	st, err := store.Open(ctx, dsn, store.WithMaxConns(4))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}

	// Fresh rows in Postgres, mirrored in the fake.
	rich, empty := "acc_"+rand.Text()[:10], "acc_"+rand.Text()[:10]
	missing := "acc_" + rand.Text()[:10]
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`INSERT INTO accounts (id, balance) VALUES ($1, 1000), ($2, 0)`,
		rich, empty); err != nil {
		t.Fatal(err)
	}
	fake := newFake()
	fake.accounts[rich], fake.accounts[empty] = 1000, 0

	type req struct{ method, path, body string }
	cases := map[string]req{
		"found":        {"GET", "/v1/accounts/" + rich, ""},
		"missing":      {"GET", "/v1/accounts/" + missing, ""},
		"funds":        {"POST", "/v1/transfers", `{"from":"` + empty + `","to":"` + rich + `","amount":5}`},
		"missing to":   {"POST", "/v1/transfers", `{"from":"` + rich + `","to":"` + missing + `","amount":5}`},
		"invalid body": {"POST", "/v1/transfers", `{"from":"` + rich + `","to":"` + rich + `","amount":-1}`},
	}
	for name, c := range cases {
		key := rand.Text()
		got := map[string]*problem{}
		codes := map[string]int{}
		for which, h := range map[string]any{"fake": fake, "real": st} {
			rec := do(t, handler(h.(Store)), c.method, c.path, key, c.body)
			codes[which] = rec.Code
			if rec.Code >= 400 {
				var p problem
				if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
					t.Fatalf("%s/%s: %v", name, which, err)
				}
				got[which] = &p
			}
		}
		if codes["fake"] != codes["real"] {
			t.Errorf("%s: fake %d, real %d", name, codes["fake"], codes["real"])
			continue
		}
		if f, r := got["fake"], got["real"]; f != nil &&
			f.Code != r.Code {
			t.Errorf("%s: fake %+v, real %+v", name, f, r)
		}
	}
}
