package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// decodeTransfer reads exactly one JSON object of at most 64 KiB
// (Technique 48). Unknown fields and trailing data are errors.
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
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
	case err != nil:
		http.Error(w, "bad json", http.StatusBadRequest)
	default:
		return true
	}
	return false
}

var errTrailing = errors.New("trailing data after JSON value")

// expectEOF wants io.EOF from one more Token call, so a second
// object or a stray } fails.
func expectEOF(dec *json.Decoder) error {
	_, err := dec.Token()
	switch {
	case err == io.EOF:
		return nil
	case err == nil:
		return errTrailing
	default:
		return err
	}
}

func encodeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.Encode(v)
}
