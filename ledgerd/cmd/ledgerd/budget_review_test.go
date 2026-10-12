package main

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"example.com/ledgerd/internal/config"
	"example.com/ledgerd/internal/httpapi"
	"example.com/ledgerd/internal/store"
)

// Technique 101's budget, checked against the real manifest: the
// kubelet's grace period starts before preStop, so preStop, the
// in-process drain, SHUTDOWN_GRACE and the trace flush must all
// fit inside terminationGracePeriodSeconds, with headroom left
// for stopping the relay and closing the pool.
func TestGraceBudgetFitsManifest(t *testing.T) {
	y, err := os.ReadFile("../../k8s/deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	num := func(re string) int {
		m := regexp.MustCompile(re).FindSubmatch(y)
		if m == nil {
			t.Fatalf("manifest has no match for %s", re)
		}
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	period := time.Duration(num(`terminationGracePeriodSeconds: (\d+)`)) * time.Second
	preStop := time.Duration(num(`sleep: \{seconds: (\d+)\}`)) * time.Second
	grace := time.Duration(num(`SHUTDOWN_GRACE\s+value: "(\d+)s"`)) * time.Second

	def, err := config.Load(func(k string) string {
		return map[string]string{"DATABASE_URL": "postgres://x"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	if grace != def.Grace {
		t.Errorf("manifest SHUTDOWN_GRACE %v, config default %v", grace, def.Grace)
	}
	sum := preStop + drain + grace + flushBudget
	t.Logf("preStop %v + drain %v + grace %v + flush %v = %v of %v",
		preStop, drain, grace, flushBudget, sum, period)
	if sum != 35*time.Second || period-sum < 5*time.Second {
		t.Errorf("budget %v leaves %v of %v for relay and pool",
			sum, period-sum, period)
	}
	// A request admitted during the drain must be able to finish
	// inside Shutdown, and the idempotency claim must outlive it.
	if def.ReqTimeout >= def.Grace {
		t.Errorf("REQUEST_TIMEOUT %v >= SHUTDOWN_GRACE %v", def.ReqTimeout, def.Grace)
	}
	if def.ReqTimeout >= httpapi.ClaimBudget || httpapi.ClaimBudget >= store.Lease {
		t.Errorf("want request %v < claim %v < lease %v",
			def.ReqTimeout, httpapi.ClaimBudget, store.Lease)
	}
}
