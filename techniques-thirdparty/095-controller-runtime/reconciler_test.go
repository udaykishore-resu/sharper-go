package operator

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	opsv1 "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/095-controller-runtime/api/v1"
)

func setup(t *testing.T) (*LedgerReconciler, *int, client.Client) {
	t.Helper()
	s := runtime.NewScheme()
	clientgoscheme.AddToScheme(s)
	opsv1.AddToScheme(s)
	l := &opsv1.Ledger{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "ledgerd", Generation: 1},
		Spec: opsv1.LedgerSpec{Replicas: 3, Image: "ledgerd:v1.4.2"}}
	writes := 0
	count := func() { writes++ }
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(l).
		WithStatusSubresource(&opsv1.Ledger{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.CreateOption) error {
				count()
				return c.Create(ctx, o, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.UpdateOption) error {
				count()
				return c.Update(ctx, o, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, o client.Object, opts ...client.SubResourceUpdateOption) error {
				count()
				return c.SubResource(sub).Update(ctx, o, opts...)
			},
		}).Build()
	return &LedgerReconciler{Client: c, Scheme: s}, &writes, c
}

var req = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ledgerd", Name: "payments"}}

// The checklist test: once converged, another pass writes nothing.
func TestConvergesThenNoWrites(t *testing.T) {
	r, writes, c := setup(t)
	for range 3 { // finalizer, then deployment + status, then nothing
		if _, err := r.Reconcile(t.Context(), req); err != nil {
			t.Fatal(err)
		}
	}
	var dep appsv1.Deployment
	if err := c.Get(t.Context(), req.NamespacedName, &dep); err != nil || *dep.Spec.Replicas != 3 {
		t.Fatalf("deployment: %v", err)
	}
	if len(dep.OwnerReferences) != 1 {
		t.Fatal("no owner reference: children won't be cleaned up")
	}
	*writes = 0
	if _, err := r.Reconcile(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if *writes != 0 {
		t.Fatalf("second pass made %d writes", *writes)
	}
}

func TestDeletedObjectDoesNotRequeue(t *testing.T) {
	r, _, c := setup(t)
	r.Reconcile(t.Context(), req) // adds the finalizer
	cleaned := 0
	r.Cleanup = func(context.Context, *opsv1.Ledger) error { cleaned++; return nil }
	var l opsv1.Ledger
	c.Get(t.Context(), req.NamespacedName, &l)
	if err := c.Delete(t.Context(), &l); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(t.Context(), req); err != nil || cleaned != 1 {
		t.Fatalf("finalize: %v cleaned=%d", err, cleaned)
	}
	if res, err := r.Reconcile(t.Context(), req); err != nil || res.RequeueAfter != 0 {
		t.Fatalf("gone object: %+v %v", res, err)
	}
}
