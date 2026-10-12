//go:build !nostdout

package main

// One line per backend; -tags nostdout drops it and everything
// it imports.
import _ "github.com/udaykishore-resu/sharper-go/techniques/099-plugin-registry/sink/stdout"
