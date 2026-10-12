// Package copied passes a Counters by value; go vet must reject it.
package copied

import obs "github.com/udaykishore-resu/sharper-go/techniques/027-zero-value"

func report(c obs.Counters) int64 { return c.Get("transfers") }

func Use() int64 {
	var c obs.Counters
	c.Add("transfers", 1)
	return report(c)
}
