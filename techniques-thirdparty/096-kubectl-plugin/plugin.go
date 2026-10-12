// Package plugin is Technique 96: kubectl-ledger, a plugin that
// behaves like kubectl. cmd/kubectl-ledger builds the binary.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func NewRoot() *cobra.Command { return newRoot() }

func newRoot() *cobra.Command {
	f := genericclioptions.NewConfigFlags(true)
	root := &cobra.Command{Use: "kubectl-ledger"}
	f.AddFlags(root.PersistentFlags())
	root.AddCommand(&cobra.Command{
		Use: "status",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := f.ToRESTConfig()
			ldr := f.ToRawKubeConfigLoader()
			ns, _, nsErr := ldr.Namespace()
			if err = errors.Join(err, nsErr); err != nil {
				return err
			}
			if raw, err := ldr.RawConfig(); err == nil {
				// Print the context first: never guess the cluster.
				fmt.Fprintf(c.OutOrStdout(), "context: %s\n",
					cmpOr(*f.Context, raw.CurrentContext))
			}
			return status(c.Context(),
				c.OutOrStdout(), cfg, ns)
		},
	})
	root.SilenceUsage = true
	return root
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func status(ctx context.Context, w io.Writer,
	cfg *rest.Config, ns string) error {
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	return statusWith(ctx, w, cs, ns, proxyReady(cs))
}

// probe checks one pod; tests swap it out.
type probe func(ctx context.Context, p corev1.Pod) error

// proxyReady calls /readyz through the API server's pod proxy, so
// no port-forward is needed. It needs get on pods/proxy.
func proxyReady(cs kubernetes.Interface) probe {
	return func(ctx context.Context, p corev1.Pod) error {
		_, err := cs.CoreV1().Pods(p.Namespace).
			ProxyGet("http", p.Name, "8080", "/readyz", nil).DoRaw(ctx)
		return err
	}
}

var errNotReady = errors.New("some pods are not ready")

// statusWith lists ledgerd pods, probes them eight at a time and
// prints a table. It fails when any pod is not ready, so the
// plugin works in a pipeline too.
func statusWith(ctx context.Context, w io.Writer,
	cs kubernetes.Interface, ns string, ready probe) error {
	pods, err := cs.CoreV1().Pods(ns).List(ctx,
		metav1.ListOptions{LabelSelector: "app=ledgerd"})
	if err != nil {
		return err
	}
	results := make([]error, len(pods.Items))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for i, p := range pods.Items {
		g.Go(func() error {
			results[i] = ready(gctx, p)
			return nil
		})
	}
	g.Wait()
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "POD\tREADY\tAGE")
	bad := false
	for i, p := range pods.Items {
		ok := results[i] == nil
		bad = bad || !ok
		fmt.Fprintf(tw, "%s\t%v\t%s\n", p.Name, ok,
			metav1.Now().Sub(p.CreationTimestamp.Time).Round(1e9))
	}
	tw.Flush()
	if bad {
		return errNotReady
	}
	return nil
}
