package operator

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	opsv1 "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/095-controller-runtime/api/v1"
)

// Review checks for Technique 95: transient API failures,
// partial progress and stale copies all converge on a later pass
// with no duplicate children, and the reconciler never loops on
// an error itself (the queue retries with backoff).

type faults struct {
	createDep, statusUpdate, ledgerUpdate int // failures left
	creates, updates, statusUpdates       int // calls seen
}

func faultySetup(t *testing.T, f *faults) (*LedgerReconciler, client.Client) {
	t.Helper()
	s := runtime.NewScheme()
	clientgoscheme.AddToScheme(s)
	opsv1.AddToScheme(s)
	l := &opsv1.Ledger{ObjectMeta: metav1.ObjectMeta{Name: "payments",
		Namespace: "ledgerd", Generation: 1},
		Spec: opsv1.LedgerSpec{Replicas: 3, Image: "ledgerd:v1.4.2"}}
	unavailable := apierrors.NewServiceUnavailable("etcd leader change")
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(l).
		WithStatusSubresource(&opsv1.Ledger{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.CreateOption) error {
				f.creates++
				if _, ok := o.(*appsv1.Deployment); ok && f.createDep > 0 {
					f.createDep--
					return unavailable
				}
				return c.Create(ctx, o, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.UpdateOption) error {
				f.updates++
				if _, ok := o.(*opsv1.Ledger); ok && f.ledgerUpdate > 0 {
					f.ledgerUpdate--
					return apierrors.NewConflict(schema.GroupResource{
						Group: "ops.example.com", Resource: "ledgers"},
						o.GetName(), errors.New("the object has been modified"))
				}
				return c.Update(ctx, o, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, o client.Object, opts ...client.SubResourceUpdateOption) error {
				f.statusUpdates++
				if f.statusUpdate > 0 {
					f.statusUpdate--
					return unavailable
				}
				return c.SubResource(sub).Update(ctx, o, opts...)
			},
		}).Build()
	return &LedgerReconciler{Client: c, Scheme: s}, c
}

// converge reconciles until a pass succeeds, as the work queue
// would, and returns how many passes failed first.
func converge(t *testing.T, r *LedgerReconciler) (failed int) {
	t.Helper()
	for range 10 {
		if _, err := r.Reconcile(t.Context(), req); err != nil {
			failed++
			continue
		}
		if _, err := r.Reconcile(t.Context(), req); err == nil {
			return failed
		}
	}
	t.Fatal("did not converge in 10 passes")
	return
}

func deployments(t *testing.T, c client.Client) int {
	var list appsv1.DeploymentList
	if err := c.List(t.Context(), &list); err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

func TestTransientFailuresConverge(t *testing.T) {
	cases := map[string]faults{
		"stale copy on finalizer add":     {ledgerUpdate: 1},
		"create fails twice":              {createDep: 2},
		"status write fails after create": {statusUpdate: 2},
		"all of them":                     {ledgerUpdate: 1, createDep: 1, statusUpdate: 1},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			r, c := faultySetup(t, &f)
			failed := converge(t, r)
			if failed == 0 {
				t.Fatal("no fault was injected")
			}
			if n := deployments(t, c); n != 1 {
				t.Fatalf("%d deployments after retries, want 1", n)
			}
			var l opsv1.Ledger
			if err := c.Get(t.Context(), req.NamespacedName, &l); err != nil {
				t.Fatal(err)
			}
			cond := meta.FindStatusCondition(l.Status.Conditions, "Available")
			if cond == nil || cond.ObservedGeneration != 1 {
				t.Fatalf("status not written: %+v", l.Status)
			}
			before := f.creates + f.updates + f.statusUpdates
			if _, err := r.Reconcile(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			if after := f.creates + f.updates + f.statusUpdates; after != before {
				t.Fatalf("converged pass made %d writes", after-before)
			}
		})
	}
}

// A conflict is returned, not retried inside Reconcile: one Update
// call per pass, so the queue's backoff (Hack 51) paces retries.
func TestConflictIsReturnedNotLooped(t *testing.T) {
	f := &faults{ledgerUpdate: 1}
	r, _ := faultySetup(t, f)
	_, err := r.Reconcile(t.Context(), req)
	if !apierrors.IsConflict(err) || f.updates != 1 {
		t.Fatalf("err %v after %d updates", err, f.updates)
	}
}

// External cleanup that keeps failing keeps the finalizer (the
// Ledger stays Terminating); it runs again on every pass until it
// succeeds, so it must be idempotent.
func TestFailingCleanupKeepsFinalizer(t *testing.T) {
	r, c := faultySetup(t, &faults{})
	converge(t, r)
	calls := 0
	r.Cleanup = func(context.Context, *opsv1.Ledger) error {
		calls++
		if calls < 3 {
			return errors.New("vendor API down")
		}
		return nil
	}
	var l opsv1.Ledger
	c.Get(t.Context(), req.NamespacedName, &l)
	if err := c.Delete(t.Context(), &l); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := r.Reconcile(t.Context(), req); err == nil {
			t.Fatal("cleanup error swallowed")
		}
		if err := c.Get(t.Context(), req.NamespacedName, &l); err != nil {
			t.Fatalf("ledger gone before cleanup succeeded: %v", err)
		}
	}
	if _, err := r.Reconcile(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(t.Context(), req.NamespacedName, &l); !apierrors.IsNotFound(err) {
		t.Fatalf("ledger still present: %v", err)
	}
	if calls != 3 {
		t.Fatalf("cleanup ran %d times, want 3", calls)
	}
}
