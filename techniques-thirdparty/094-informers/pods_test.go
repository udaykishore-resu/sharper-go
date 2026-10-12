package guard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
)

func pod(ns, name, app string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: ns, Name: name, Labels: map[string]string{"app": app},
		ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
	}}
}

func TestListerReadsScopedCache(t *testing.T) {
	cs := fake.NewClientset(
		pod("ledgerd", "ledgerd-1", "ledgerd"),
		pod("ledgerd", "ledgerd-2", "ledgerd"),
		pod("ledgerd", "redis-0", "redis"),
		pod("other", "ledgerd-x", "ledgerd"),
	)
	lister, err := podLister(t.Context(), cs)
	if err != nil {
		t.Fatal(err)
	}
	pods, err := lister.Pods("ledgerd").List(labels.Everything())
	if err != nil || len(pods) != 2 {
		t.Fatalf("pods = %d, %v", len(pods), err)
	}
	for _, p := range pods {
		if len(p.ManagedFields) != 0 {
			t.Fatal("transform did not drop managed fields")
		}
	}
	// Reads come from memory: no new LIST reaches the server.
	before := len(cs.Actions())
	for range 100 {
		lister.Pods("ledgerd").List(labels.Everything())
	}
	if len(cs.Actions()) != before {
		t.Fatal("lister reads hit the API server")
	}
}
