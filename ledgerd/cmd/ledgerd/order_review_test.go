package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

// The middleware order is part of the design (Techniques 47, 88
// and 90): layers that copy r sit outside the layers that read
// r.Pattern, and Recover sits inside metrics. The book prints
// this list in Parts V, IX and XI; this test pins serve.go to it.
var canonicalOrder = []string{
	"RequestID", "Auth", "Deadline", "NewMiddleware",
	"AccessLog", "Wrap", "Recover", "LimitBody",
}

// Listener addresses the book states: the API on :8080, metrics
// on the pod IP at :9090, pprof and expvar on loopback only.
var wantListeners = map[string]string{
	`":8080"`:          "root",
	`":9090"`:          "promhttp.HandlerFor",
	`"127.0.0.1:6060"`: "obs.DebugMux",
}

func TestServeMiddlewareOrderAndListeners(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "serve.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	listeners := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch name := exprName(call.Fun); name {
		case "httpapi.Chain":
			for _, a := range call.Args[1:] {
				if c, ok := a.(*ast.CallExpr); ok {
					a = c.Fun
				}
				s := exprName(a)
				order = append(order, s[strings.LastIndex(s, ".")+1:])
			}
		case "httpapi.NewServer", "internal":
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Errorf("%s: address is not a literal", name)
				return true
			}
			h := call.Args[1]
			if c, ok := h.(*ast.CallExpr); ok {
				h = c.Fun
			}
			listeners[lit.Value] = exprName(h)
		}
		return true
	})
	if !slices.Equal(order, canonicalOrder) {
		t.Errorf("serve.go middleware order\n got %v\nwant %v", order, canonicalOrder)
	}
	for addr, h := range wantListeners {
		if listeners[addr] != h {
			t.Errorf("listener %s serves %q, want %q", addr, listeners[addr], h)
		}
	}
	if len(listeners) != len(wantListeners) {
		t.Errorf("listeners = %v", listeners)
	}
}

func exprName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprName(e.X) + "." + e.Sel.Name
	}
	return ""
}
