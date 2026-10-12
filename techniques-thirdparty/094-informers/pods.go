// Package guard is Technique 94: read Kubernetes state from an
// informer's local cache instead of polling the API server.
package guard

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
)

func podLister(ctx context.Context, cs kubernetes.Interface,
) (corelisters.PodLister, error) {
	f := informers.NewSharedInformerFactoryWithOptions(
		cs, 10*time.Minute,
		informers.WithNamespace("ledgerd"),
		informers.WithTweakListOptions(
			func(o *metav1.ListOptions) {
				o.LabelSelector = "app=ledgerd"
			}),
		informers.WithTransform(dropManaged),
	)
	lister := f.Core().V1().Pods().Lister()
	f.StartWithContext(ctx)
	res := f.WaitForCacheSyncWithContext(ctx)
	if err := res.AsError(); err != nil {
		return nil, fmt.Errorf("cache sync: %w", err)
	}
	return lister, nil
}

// dropManaged clears server-side apply bookkeeping most
// controllers never read, so the cache holds less.
func dropManaged(obj any) (any, error) {
	if m, err := meta.Accessor(obj); err == nil {
		m.SetManagedFields(nil)
	}
	return obj, nil
}
