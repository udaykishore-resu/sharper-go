// Package statements is Technique 53: os.Root for traversal-safe
// file access.
package statements

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
)

type Statements struct{ root *os.Root }

// Open the root once at startup; it is safe for concurrent use.
func New(dir string) (*Statements, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &Statements{root: root}, nil
}

func (s *Statements) Get(w http.ResponseWriter,
	r *http.Request) {
	name := r.PathValue("id") + "/" +
		r.PathValue("month") + ".csv"
	f, err := s.root.Open(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/csv")
	io.Copy(w, f)
}
