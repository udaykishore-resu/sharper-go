// Package toolchainline is Technique 1: go.mod's go and toolchain
// lines, and the CI check that keeps the builder image honest.
//
// The tests drive the real go command in throwaway modules. They
// need no network: every download either comes from a local file
// proxy the test writes, or is expected to fail.
package toolchainline
