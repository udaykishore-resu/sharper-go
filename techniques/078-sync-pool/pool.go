// Package respond is Technique 78: a sync.Pool for response
// buffers, with a cap on what goes back.
package respond

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
)

var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

const maxPooled = 64 << 10

func writeJSON(w http.ResponseWriter, v any) error {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= maxPooled {
			bufPool.Put(buf)
		}
	}()
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func writeJSONPlain(w http.ResponseWriter, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
