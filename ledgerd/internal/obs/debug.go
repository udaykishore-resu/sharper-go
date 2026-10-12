package obs

import (
	"expvar"
	"net/http"
	"net/http/pprof"
)

// Facts about this one process, read with curl during an
// incident (Technique 89).
var (
	ConfigVersion = expvar.NewString("config_version")
	OutboxLastRun = expvar.NewString("outbox_last_run")
	Rejects       = expvar.NewMap("transfer_rejects")
)

func init() {
	expvar.Publish("build", expvar.Func(
		func() any { return BuildInfo() }))
}

// DebugMux serves pprof and expvar on their own mux, never on
// http.DefaultServeMux (Techniques 73 and 89). ledgerd binds it
// to 127.0.0.1:6060, reachable only through port-forward. It
// leaves out pprof.Cmdline: flags are the usual place a secret
// leaks from, and nothing in a profile workflow needs them.
func DebugMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/vars", expvar.Handler())
	return mux
}
