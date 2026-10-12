// Package obs is Technique 89: expvar for facts about one process,
// right now.
package obs

import (
	"expvar"
	"net/http"
	"sync/atomic"
)

// breaker is a stand-in for a circuit breaker to the fraud
// service. State reads an atomic, never a lock.
type breaker struct{ open atomic.Bool }

func (b *breaker) State() string {
	if b.open.Load() {
		return "open"
	}
	return "closed"
}

var fraudBreaker = &breaker{}

var (
	ConfigVersion = expvar.NewString("config_version")
	OutboxLastRun = expvar.NewString("outbox_last_run")
	Rejects       = expvar.NewMap("transfer_rejects")
)

func init() {
	expvar.Publish("breaker_fraud", expvar.Func(
		func() any { return fraudBreaker.State() }))
}

func MountVars(debug *http.ServeMux) {
	debug.Handle("/debug/vars", expvar.Handler())
}
