// Package goprivate is Technique 9: GOPRIVATE, GOPROXY and
// GOFLAGS for corporate setups.
//
// goenv.sh is the printed script. The tests run the go command
// against local proxies (an httptest server and a file:// proxy)
// to check what each setting really does; nothing leaves the
// machine.
package goprivate
