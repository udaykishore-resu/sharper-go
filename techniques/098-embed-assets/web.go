// Package web is Technique 98: static assets and templates
// embedded in the binary.
package web

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed static templates/*.html
var files embed.FS

var pages = template.Must(
	template.ParseFS(files, "templates/*.html"))

func Static() http.Handler {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err) // only if the path is wrong
	}
	return http.FileServerFS(sub)
}

// Render executes into a buffer first, so a template error is a
// clean 500 instead of half a page with a 200.
func Render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := pages.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
