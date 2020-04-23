/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusterapi

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
)

func newTaintFromInterface(t interface{}) corev1.Taint {
	taint := corev1.Taint{}
	if tt, ok := t.(map[string]interface{}); ok {
		if tmap, found, _ := unstructured.NestedMap(tt); found {
			if val, ok := tmap["key"].(string); ok {
				taint.Key = val
			}
			if val, ok := tmap["effect"].(string); ok {
				taint.Effect = corev1.TaintEffect(val)
			}
			if val, ok := tmap["value"].(string); ok {
				taint.Value = val
			}
			if val, ok := tmap["timeAdded"].(string); ok {
				ta := time.Time{}
				ta.UnmarshalText([]byte(val))
				nta := metav1.NewTime(ta)
				taint.TimeAdded = &nta
			}
		}
	}
	return taint
}

func newMachineDeploymentFromUnstructured(u *unstructured.Unstructured) *MachineDeployment {
	machineDeployment := MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       u.GetKind(),
			APIVersion: u.GetAPIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              u.GetName(),
			Namespace:         u.GetNamespace(),
			UID:               u.GetUID(),
			Labels:            u.GetLabels(),
			Annotations:       u.GetAnnotations(),
			OwnerReferences:   u.GetOwnerReferences(),
			DeletionTimestamp: u.GetDeletionTimestamp(),
		},
		Spec:   MachineDeploymentSpec{},
		Status: MachineDeploymentStatus{},
	}

	machineSpec, found, err := unstructured.NestedMap(u.Object, "spec", "template", "spec")
	if err == nil && found {
		machineSpecUnstructured := unstructured.Unstructured{Object: machineSpec}
		machineDeployment.Spec.Template.Spec.Labels = machineSpecUnstructured.GetLabels()
		taints, _, _ := unstructured.NestedSlice(machineSpec, "taints")
		if taints != nil {
			taintobjlist := make([]corev1.Taint, len(taints))
			for i, t := range taints {
				taintobjlist[i] = newTaintFromInterface(t)
			}
			machineDeployment.Spec.Template.Spec.Taints = taintobjlist
		}
	}

	if replicas, found, err := unstructured.NestedInt64(u.Object, "spec", "replicas"); err == nil && found {
		machineDeployment.Spec.Replicas = pointer.Int32Ptr(int32(replicas))
	}

	if replicas, found, err := unstructured.NestedInt64(u.Object, "status", "replicas"); err == nil && found {
		machineDeployment.Status.Replicas = int32(replicas)
	}

	return &machineDeployment
}

func newMachineSetFromUnstructured(u *unstructured.Unstructured) *MachineSet {
	machineSet := MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       u.GetKind(),
			APIVersion: u.GetAPIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              u.GetName(),
			Namespace:         u.GetNamespace(),
			UID:               u.GetUID(),
			Labels:            u.GetLabels(),
			Annotations:       u.GetAnnotations(),
			OwnerReferences:   u.GetOwnerReferences(),
			DeletionTimestamp: u.GetDeletionTimestamp(),
		},
		Spec:   MachineSetSpec{},
		Status: MachineSetStatus{},
	}

	machineSpec, found, err := unstructured.NestedMap(u.Object, "spec", "template", "spec")
	if err == nil && found {
		machineSpecUnstructured := unstructured.Unstructured{Object: machineSpec}
		machineSet.Spec.Template.Spec.Labels = machineSpecUnstructured.GetLabels()
		taints, _, _ := unstructured.NestedSlice(machineSpec, "taints")
		if taints != nil {
			taintobjlist := make([]corev1.Taint, len(taints))
			for i, t := range taints {
				taintobjlist[i] = newTaintFromInterface(t)
			}
			machineSet.Spec.Template.Spec.Taints = taintobjlist
		}
	}

	if replicas, found, err := unstructured.NestedInt64(u.Object, "spec", "replicas"); err == nil && found {
		machineSet.Spec.Replicas = pointer.Int32Ptr(int32(replicas))
	}

	if replicas, found, err := unstructured.NestedInt64(u.Object, "status", "replicas"); err == nil && found {
		machineSet.Status.Replicas = int32(replicas)
	}

	return &machineSet
}

func newMachineFromUnstructured(u *unstructured.Unstructured) *Machine {
	machine := Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       u.GetKind(),
			APIVersion: u.GetAPIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              u.GetName(),
			Namespace:         u.GetNamespace(),
			UID:               u.GetUID(),
			Labels:            u.GetLabels(),
			Annotations:       u.GetAnnotations(),
			OwnerReferences:   u.GetOwnerReferences(),
			ClusterName:       u.GetClusterName(),
			DeletionTimestamp: u.GetDeletionTimestamp(),
		},
		Spec:   MachineSpec{},
		Status: MachineStatus{},
	}

	if providerID, _, _ := unstructured.NestedString(u.Object, "spec", "providerID"); providerID != "" {
		machine.Spec.ProviderID = pointer.StringPtr(providerID)
	}

	nodeRef := corev1.ObjectReference{}

	if nodeRefKind, _, _ := unstructured.NestedString(u.Object, "status", "nodeRef", "kind"); nodeRefKind != "" {
		nodeRef.Kind = nodeRefKind
	}

	if nodeRefName, _, _ := unstructured.NestedString(u.Object, "status", "nodeRef", "name"); nodeRefName != "" {
		nodeRef.Name = nodeRefName
	}

	if nodeRef.Name != "" || nodeRef.Kind != "" {
		machine.Status.NodeRef = &nodeRef
	}

	if errorMessage, _, _ := unstructured.NestedString(u.Object, "status", "errorMessage"); errorMessage != "" {
		machine.Status.ErrorMessage = pointer.StringPtr(errorMessage)
	}

	return &machine
}

func newUnstructuredFromMachineSet(m *MachineSet) *unstructured.Unstructured {
	u := unstructured.Unstructured{}

	u.SetAPIVersion(m.APIVersion)
	u.SetAnnotations(m.Annotations)
	u.SetKind(m.Kind)
	u.SetLabels(m.Labels)
	u.SetName(m.Name)
	u.SetNamespace(m.Namespace)
	u.SetOwnerReferences(m.OwnerReferences)
	u.SetUID(m.UID)
	u.SetDeletionTimestamp(m.DeletionTimestamp)

	if m.Spec.Replicas != nil {
		unstructured.SetNestedField(u.Object, int64(*m.Spec.Replicas), "spec", "replicas")
	}
	unstructured.SetNestedField(u.Object, int64(m.Status.Replicas), "status", "replicas")

	return &u
}

func newUnstructuredFromMachineDeployment(m *MachineDeployment) *unstructured.Unstructured {
	u := unstructured.Unstructured{}

	u.SetAPIVersion(m.APIVersion)
	u.SetAnnotations(m.Annotations)
	u.SetKind(m.Kind)
	u.SetLabels(m.Labels)
	u.SetName(m.Name)
	u.SetNamespace(m.Namespace)
	u.SetOwnerReferences(m.OwnerReferences)
	u.SetUID(m.UID)
	u.SetDeletionTimestamp(m.DeletionTimestamp)

	if m.Spec.Replicas != nil {
		unstructured.SetNestedField(u.Object, int64(*m.Spec.Replicas), "spec", "replicas")
	}
	unstructured.SetNestedField(u.Object, int64(m.Status.Replicas), "status", "replicas")

	return &u
}

func newUnstructuredFromMachine(m *Machine) *unstructured.Unstructured {
	u := unstructured.Unstructured{}

	u.SetAPIVersion(m.APIVersion)
	u.SetAnnotations(m.Annotations)
	u.SetKind(m.Kind)
	u.SetLabels(m.Labels)
	u.SetName(m.Name)
	u.SetNamespace(m.Namespace)
	u.SetOwnerReferences(m.OwnerReferences)
	u.SetUID(m.UID)
	u.SetDeletionTimestamp(m.DeletionTimestamp)

	if m.Spec.ProviderID != nil && *m.Spec.ProviderID != "" {
		unstructured.SetNestedField(u.Object, *m.Spec.ProviderID, "spec", "providerID")
	}

	if m.Status.NodeRef != nil {
		if m.Status.NodeRef.Kind != "" {
			unstructured.SetNestedField(u.Object, m.Status.NodeRef.Kind, "status", "nodeRef", "kind")
		}
		if m.Status.NodeRef.Name != "" {
			unstructured.SetNestedField(u.Object, m.Status.NodeRef.Name, "status", "nodeRef", "name")
		}
	}

	if m.Status.ErrorMessage != nil {
		unstructured.SetNestedField(u.Object, *m.Status.ErrorMessage, "status", "errorMessage")
	}

	return &u
}
