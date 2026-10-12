// Package gowork is Technique 3: a go.work file in the folder that
// holds two checkouts, so ledgerd builds against a local moneykit
// without a replace line in either go.mod.
//
// The test builds the layout from the book in a temp folder:
//
//	src/
//	├── go.work      (made by go work init, never committed)
//	├── ledgerd/     requires example.com/moneykit v0.3.0
//	└── moneykit/    the local, unreleased change
package gowork
