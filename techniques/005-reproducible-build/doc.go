// Package reprobuild is Technique 5: reproducible,
// version-stamped binaries.
//
// build.sh is the printed script. The tests build a tiny
// ./cmd/ledgerd in throwaway folders and Git repositories and
// compare SHA-256 hashes, so every claim about "same bytes" is
// checked on the toolchain running the test.
package reprobuild
