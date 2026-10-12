// Package operator is Technique 95: a reconciler that reads the
// world as it is and nudges it one step, safe to run twice.
package operator

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	opsv1 "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/095-controller-runtime/api/v1"
)

const finalizer = "ops.example.com/cleanup"

type LedgerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Cleanup releases what lives outside the cluster. It must be
	// idempotent: it runs again until it succeeds.
	Cleanup func(ctx context.Context, l *opsv1.Ledger) error
}

func (r *LedgerReconciler) Reconcile(ctx context.Context,
	req ctrl.Request) (ctrl.Result, error) {
	var l opsv1.Ledger
	err := r.Get(ctx, req.NamespacedName, &l)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !l.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, &l)
	}
	if controllerutil.AddFinalizer(&l, finalizer) {
		return ctrl.Result{}, r.Update(ctx, &l)
	}
	dep, err := r.ensureDeployment(ctx, &l)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.setStatus(ctx, &l, dep)
}

func (r *LedgerReconciler) finalize(ctx context.Context,
	l *opsv1.Ledger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(l, finalizer) {
		return ctrl.Result{}, nil
	}
	if r.Cleanup != nil {
		if err := r.Cleanup(ctx, l); err != nil {
			return ctrl.Result{}, err
		}
	}
	controllerutil.RemoveFinalizer(l, finalizer)
	return ctrl.Result{}, r.Update(ctx, l)
}

// ensureDeployment sets only the fields it owns, plus the owner
// reference, so CreateOrUpdate writes only on a real change.
func (r *LedgerReconciler) ensureDeployment(ctx context.Context,
	l *opsv1.Ledger) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: l.Name, Namespace: l.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		labels := map[string]string{"app": "ledgerd", "ledger": l.Name}
		dep.Spec.Replicas = &l.Spec.Replicas
		if dep.Spec.Selector == nil { // immutable after create
			dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		}
		dep.Spec.Template.Labels = labels
		if len(dep.Spec.Template.Spec.Containers) == 0 {
			dep.Spec.Template.Spec.Containers = []corev1.Container{{Name: "ledgerd"}}
		}
		dep.Spec.Template.Spec.Containers[0].Image = l.Spec.Image
		return controllerutil.SetControllerReference(l, dep, r.Scheme)
	})
	return dep, err
}

// setStatus writes a condition with ObservedGeneration, and
// writes nothing when nothing changed.
func (r *LedgerReconciler) setStatus(ctx context.Context,
	l *opsv1.Ledger, dep *appsv1.Deployment) (ctrl.Result, error) {
	cond := metav1.Condition{Type: "Available", Status: metav1.ConditionFalse,
		Reason: "Progressing", ObservedGeneration: l.Generation}
	if dep.Status.AvailableReplicas >= l.Spec.Replicas {
		cond.Status, cond.Reason = metav1.ConditionTrue, "ReplicasReady"
	}
	if !meta.SetStatusCondition(&l.Status.Conditions, cond) {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, r.Status().Update(ctx, l)
}

// SetupWithManager filters out status-only updates, or the
// controller's own status write would wake it again.
func (r *LedgerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opsv1.Ledger{}, builder.WithPredicates(
			predicate.GenerationChangedPredicate{})).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
