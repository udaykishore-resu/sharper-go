// Package decode is Technique 48: http.MaxBytesReader and a
// strict JSON body.
package decode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func decodeTransfer(w http.ResponseWriter,
	r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil {
		err = expectEOF(dec)
	}
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		http.Error(w, "body too large", 413)
	case err != nil:
		http.Error(w, "bad json", 400)
	default:
		return true
	}
	return false
}

// expectEOF calls dec.Token() once more and wants io.EOF.
func expectEOF(dec *json.Decoder) error {
	_, err := dec.Token()
	switch {
	case err == io.EOF:
		return nil
	case err == nil:
		return errors.New("trailing data after JSON value")
	default:
		return err
	}
}

// LimitBody is the 1 MB global ceiling; handlers tighten it.
func LimitBody(n int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, n)
		next.ServeHTTP(w, r)
	})
}
