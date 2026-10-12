package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Review checks for Technique 96, run against the built binary:
// its name, argument forwarding, kubeconfig and --context
// selection, exit codes, and the confirmation before a write.
// kubectl itself is not installed here, so kubectl's PATH lookup
// of "kubectl-ledger" is not exercised.

// apiServer fakes the few API paths the plugin calls and records
// every request it sees.
type apiServer struct {
	*httptest.Server
	mu       sync.Mutex
	seen     []string
	notReady string // pod whose /readyz fails
}

func newAPIServer(t *testing.T, notReady string) *apiServer {
	a := &apiServer{notReady: notReady}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.seen = append(a.seen, r.Method+" "+r.URL.Path)
		a.mu.Unlock()
		parts := strings.Split(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/pods"):
			ns := parts[4]
			list := corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}, Items: []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "ledgerd-a", Namespace: ns}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ledgerd-b", Namespace: ns}},
			}}
			json.NewEncoder(w).Encode(list)
		case strings.HasSuffix(r.URL.Path, "/proxy/readyz"):
			if a.notReady != "" && strings.Contains(r.URL.Path, a.notReady) {
				http.Error(w, "draining", 503)
				return
			}
			io.WriteString(w, "ok")
		case r.Method == "PATCH" && strings.Contains(r.URL.Path, "/deployments/ledgerd"):
			json.NewEncoder(w).Encode(appsv1.Deployment{
				TypeMeta:   metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "ledgerd", Namespace: parts[5]}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(a.Close)
	return a
}

func (a *apiServer) requests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.seen...)
}

func buildPlugin(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "kubectl-ledger")
	out, err := exec.Command("go", "build", "-o", bin, "./cmd/kubectl-ledger").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func kubeconfig(t *testing.T, prod, staging string) string {
	cfg := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: prod
  cluster: {server: %q}
- name: staging
  cluster: {server: %q}
users:
- name: oncall
  user: {token: not-a-real-token}
contexts:
- name: prod
  context: {cluster: prod, user: oncall, namespace: ledgerd}
- name: staging
  context: {cluster: staging, user: oncall, namespace: ledgerd-staging}
current-context: prod
`, prod, staging)
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

type result struct {
	code           int
	stdout, stderr string
}

func run(t *testing.T, bin, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{code, out.String(), errb.String()}
}

func TestPluginBinary(t *testing.T) {
	bin := buildPlugin(t)
	if filepath.Base(bin) != "kubectl-ledger" {
		t.Fatal("kubectl finds plugins by the kubectl- prefix")
	}
	prod, staging := newAPIServer(t, ""), newAPIServer(t, "ledgerd-b")
	kc := kubeconfig(t, prod.URL, staging.URL)

	// Default: current-context prod, its namespace, exit 0.
	r := run(t, bin, "", "status", "--kubeconfig", kc)
	if r.code != 0 || !strings.HasPrefix(r.stdout, "context: prod\n") {
		t.Fatalf("status on prod: %+v", r)
	}
	if got := prod.requests(); len(got) == 0 || got[0] != "GET /api/v1/namespaces/ledgerd/pods" {
		t.Fatalf("prod saw %v", got)
	}

	// --context and -n are forwarded; a pod not ready exits 1.
	r = run(t, bin, "", "status", "--kubeconfig", kc,
		"--context", "staging", "-n", "blue")
	if r.code != 1 || !strings.HasPrefix(r.stdout, "context: staging\n") ||
		!strings.Contains(r.stderr, "some pods are not ready") {
		t.Fatalf("status on staging: %+v", r)
	}
	if got := staging.requests(); got[0] != "GET /api/v1/namespaces/blue/pods" {
		t.Fatalf("staging saw %v", got)
	}

	// A broken kubeconfig fails with exit 1 and a message.
	r = run(t, bin, "", "status", "--kubeconfig", filepath.Join(t.TempDir(), "nope"))
	if r.code != 1 || r.stderr == "" {
		t.Fatalf("missing kubeconfig: %+v", r)
	}
}

func patches(a *apiServer) int {
	n := 0
	for _, s := range a.requests() {
		if strings.HasPrefix(s, "PATCH ") {
			n++
		}
	}
	return n
}

// The one write asks first, after naming the context, and only
// "y" goes ahead; --yes skips the prompt.
func TestRestartConfirms(t *testing.T) {
	bin := buildPlugin(t)
	prod, staging := newAPIServer(t, ""), newAPIServer(t, "")
	kc := kubeconfig(t, prod.URL, staging.URL)

	for _, answer := range []string{"", "n\n", "yes please\n"} {
		r := run(t, bin, answer, "restart", "--kubeconfig", kc)
		if r.code != 1 || !strings.HasPrefix(r.stdout, "context: prod\n") ||
			!strings.Contains(r.stdout, "restart deployment ledgerd in ledgerd? [y/N]") {
			t.Fatalf("answer %q: %+v", answer, r)
		}
	}
	if n := patches(prod); n != 0 {
		t.Fatalf("%d writes without a yes", n)
	}
	if r := run(t, bin, "y\n", "restart", "--kubeconfig", kc); r.code != 0 || patches(prod) != 1 {
		t.Fatalf("answer y: %+v, %d patches", r, patches(prod))
	}
	r := run(t, bin, "", "restart", "--kubeconfig", kc, "--context", "staging", "--yes")
	if r.code != 0 || patches(staging) != 1 || strings.Contains(r.stdout, "[y/N]") {
		t.Fatalf("--yes: %+v", r)
	}
}

// restartWith stamps the pod template, as kubectl rollout restart.
func TestRestartStampsTemplate(t *testing.T) {
	cs := fake.NewClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ledgerd", Name: "ledgerd"}})
	now := metav1.Now().Time
	if err := restartWith(t.Context(), cs, "ledgerd", now); err != nil {
		t.Fatal(err)
	}
	d, _ := cs.AppsV1().Deployments("ledgerd").Get(t.Context(), "ledgerd", metav1.GetOptions{})
	if d.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("template not stamped: %+v", d.Spec.Template.Annotations)
	}
}
