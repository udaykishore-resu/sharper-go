package synctestdemo

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/udaykishore-resu/sharper-go/techniques/064-synctest/hold"
)

func TestHoldExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		h := hold.New(15 * time.Minute)
		go h.Sweep(ctx, time.Minute)
		h.Place("tr_1")
		time.Sleep(14 * time.Minute)
		synctest.Wait()
		if !h.Active("tr_1") {
			t.Fatal("expired too early")
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if h.Active("tr_1") {
			t.Fatal("still active after 15m")
		}
	})
}
