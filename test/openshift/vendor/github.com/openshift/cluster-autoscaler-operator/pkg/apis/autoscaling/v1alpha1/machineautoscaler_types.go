package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&MachineAutoscaler{}, &MachineAutoscalerList{})
}

// MachineAutoscalerSpec defines the desired state of MachineAutoscaler
type MachineAutoscalerSpec struct {
	MinReplicas    int32                       `json:"minReplicas"`
	MaxReplicas    int32                       `json:"maxReplicas"`
	ScaleTargetRef CrossVersionObjectReference `json:"scaleTargetRef"`
}

// MachineAutoscalerStatus defines the observed state of MachineAutoscaler
type MachineAutoscalerStatus struct {
	// TODO: Add status fields.
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineAutoscaler is the Schema for the machineautoscalers API
// +k8s:openapi-gen=true
type MachineAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineAutoscalerSpec   `json:"spec,omitempty"`
	Status MachineAutoscalerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineAutoscalerList contains a list of MachineAutoscaler
type MachineAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineAutoscaler `json:"items"`
}

// CrossVersionObjectReference identifies another object by name, API version,
// and kind.
type CrossVersionObjectReference struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	APIVersion string `json:"apiVersion,omitempty"`
}
