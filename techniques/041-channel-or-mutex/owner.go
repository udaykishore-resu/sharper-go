package balances

// owned is the channel version from the review: one goroutine
// owns the map, and callers send requests with a reply channel.
// It exists so the benchmark compares equivalent workloads.
type owned struct {
	reqs chan req
	quit chan struct{}
}

type req struct {
	id    string
	d     int64
	reply chan int64
}

func newOwned() *owned {
	o := &owned{reqs: make(chan req), quit: make(chan struct{})}
	go func() {
		m := map[string]int64{}
		for {
			select {
			case r := <-o.reqs:
				m[r.id] += r.d
				r.reply <- m[r.id]
			case <-o.quit:
				return
			}
		}
	}()
	return o
}

// Add applies d and returns the new balance.
func (o *owned) Add(id string, d int64) int64 {
	reply := make(chan int64, 1)
	o.reqs <- req{id, d, reply}
	return <-reply
}

func (o *owned) Get(id string) int64 { return o.Add(id, 0) }

func (o *owned) Close() { close(o.quit) }
