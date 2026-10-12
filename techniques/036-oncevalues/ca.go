// Package tlsca is Technique 36: sync.OnceValues instead of a
// Once, a var and an error field.
package tlsca

import (
	"crypto/x509"
	_ "embed"
	"errors"
	"sync"
)

//go:embed testdata/partner-ca.pem
var caPEM []byte

func readCA() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("tlsca: no certificates in bundle")
	}
	return pool, nil
}

// The bytes can't change, so a cached parse error is correct.
var loadCA = sync.OnceValues(readCA)
