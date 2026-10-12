// Package v1 is the ops.example.com/v1 API with the Ledger kind.
// DeepCopy methods are hand-written here; controller-gen would
// generate them in a real operator.
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "ops.example.com", Version: "v1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &Ledger{}, &LedgerList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

type LedgerSpec struct {
	Replicas int32  `json:"replicas"`
	Image    string `json:"image"`
}

type LedgerStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type Ledger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LedgerSpec   `json:"spec"`
	Status            LedgerStatus `json:"status,omitempty"`
}

type LedgerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ledger `json:"items"`
}

func (in *Ledger) DeepCopyInto(out *Ledger) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		for i := range in.Status.Conditions {
			in.Status.Conditions[i].DeepCopyInto(&out.Status.Conditions[i])
		}
	}
}

func (in *Ledger) DeepCopy() *Ledger {
	if in == nil {
		return nil
	}
	out := new(Ledger)
	in.DeepCopyInto(out)
	return out
}

func (in *Ledger) DeepCopyObject() runtime.Object { return in.DeepCopy() }

func (in *LedgerList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(LedgerList)
	*out = *in
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Ledger, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}
