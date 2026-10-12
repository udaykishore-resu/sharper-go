// Package internalpkg is Technique 28: the go command's internal/
// rule, checked by building throwaway module trees in a test.
//
// The rule (go help internal): an import of a path containing the
// element "internal" is allowed only from code inside the tree
// rooted at the parent of that "internal" directory. The check
// compares import paths, not modules, so a nested module whose path
// sits under the parent passes, and replace or go.work does not
// open the wall for an unrelated path.
package internalpkg
