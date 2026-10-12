package guard

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// Review checks for Technique 94.

// The API server refuses the LIST: podLister returns an error
// naming the sync, and no lister, so nothing can read an empty
// cache as "every pod was deleted".
func TestSyncFailureReturnsNoLister(t *testing.T) {
	cs := fake.NewClientset(pod("ledgerd", "ledgerd-1", "ledgerd"))
	cs.PrependReactor("list", "pods",
		func(k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, errors.New("apiserver unavailable")
		})
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	lister, err := podLister(ctx, cs)
	if err == nil || lister != nil || !strings.Contains(err.Error(), "cache sync") {
		t.Fatalf("lister %v, err %v", lister, err)
	}
}

// The watch drops (an API server restart, a load balancer idle
// cut): the reflector opens a new one, and later changes still
// reach the cache. When the context ends, the informer's
// goroutines exit.
func TestWatchReconnectsAndStops(t *testing.T) {
	base := runtime.NumGoroutine()
	cs := fake.NewClientset(pod("ledgerd", "ledgerd-1", "ledgerd"))
	var (
		mu      sync.Mutex
		watches []watch.Interface
	)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	cs.PrependWatchReactor("pods",
		func(a k8stesting.Action) (bool, watch.Interface, error) {
			w, err := cs.Tracker().Watch(gvr, a.GetNamespace())
			mu.Lock()
			watches = append(watches, w)
			mu.Unlock()
			return true, w, err
		})
	ctx, cancel := context.WithCancel(t.Context())
	lister, err := podLister(ctx, cs)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(watches) == 1 })
	mu.Lock()
	watches[0].Stop() // the connection drops
	mu.Unlock()
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(watches) >= 2 })

	_, err = cs.CoreV1().Pods("ledgerd").Create(ctx,
		pod("ledgerd", "ledgerd-2", "ledgerd"), metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		ps, _ := lister.Pods("ledgerd").List(labels.Everything())
		return len(ps) == 2
	})

	cancel()
	waitFor(t, func() bool { return runtime.NumGoroutine() <= base+2 })
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
