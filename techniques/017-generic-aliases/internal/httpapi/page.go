// internal/httpapi/page.go: the old name stays (Technique 17).
package httpapi

import "github.com/udaykishore-resu/sharper-go/techniques/017-generic-aliases/internal/page"

// Page is kept so existing callers compile.
//
// Deprecated: use page.Page.
type Page[T any] = page.Page[T]

// A short name, not a new type.
type Set[T comparable] = map[T]struct{}
