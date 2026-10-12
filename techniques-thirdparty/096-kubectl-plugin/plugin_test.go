package plugin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStatusTable(t *testing.T) {
	cs := fake.NewClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ledgerd", Name: "ledgerd-a", Labels: map[string]string{"app": "ledgerd"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ledgerd", Name: "ledgerd-b", Labels: map[string]string{"app": "ledgerd"}}},
	)
	var out bytes.Buffer
	probe := func(_ context.Context, p corev1.Pod) error {
		if p.Name == "ledgerd-b" {
			return errors.New("503")
		}
		return nil
	}
	err := statusWith(t.Context(), &out, cs, "ledgerd", probe)
	if !errors.Is(err, errNotReady) {
		t.Fatalf("err = %v; must exit non-zero when a pod is not ready", err)
	}
	if !strings.Contains(out.String(), "ledgerd-a  true") || !strings.Contains(out.String(), "ledgerd-b  false") {
		t.Fatalf("table:\n%s", &out)
	}
}

// AddFlags gives the plugin kubectl's own flags.
func TestKubectlFlags(t *testing.T) {
	root := newRoot()
	for _, f := range []string{"context", "namespace", "kubeconfig", "request-timeout"} {
		if root.PersistentFlags().Lookup(f) == nil {
			t.Errorf("missing --%s", f)
		}
	}
}
