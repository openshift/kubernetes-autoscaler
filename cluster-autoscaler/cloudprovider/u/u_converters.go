package u

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime "k8s.io/apimachinery/pkg/runtime"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

func newMachineDeploymentFromUnstructured(u *unstructured.Unstructured) *MachineDeployment {
	machineDeployment := MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       u.GetKind(),
			APIVersion: u.GetAPIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            u.GetName(),
			GenerateName:    "",
			Namespace:       u.GetNamespace(),
			SelfLink:        "",
			UID:             u.GetUID(),
			ResourceVersion: "",
			Generation:      0,
			CreationTimestamp: metav1.Time{
				Time: time.Time{},
			},
			DeletionTimestamp: &metav1.Time{
				Time: time.Time{},
			},
			DeletionGracePeriodSeconds: nil,
			Labels:                     u.GetLabels(),
			Annotations:                u.GetAnnotations(),
			OwnerReferences:            u.GetOwnerReferences(),
			Initializers: &metav1.Initializers{
				Pending: nil,
				Result: &metav1.Status{
					TypeMeta: metav1.TypeMeta{
						Kind:       "",
						APIVersion: "",
					},
					ListMeta: metav1.ListMeta{
						SelfLink:        "",
						ResourceVersion: "",
						Continue:        "",
					},
					Status:  "",
					Message: "",
					Reason:  "",
					Details: &metav1.StatusDetails{
						Name:              "",
						Group:             "",
						Kind:              "",
						UID:               "",
						Causes:            nil,
						RetryAfterSeconds: 0,
					},
					Code: 0,
				},
			},
			Finalizers:    nil,
			ClusterName:   "",
			ManagedFields: nil,
		},
		Spec: MachineDeploymentSpec{
			Replicas: nil,
			Selector: metav1.LabelSelector{
				MatchLabels:      nil,
				MatchExpressions: nil,
			},
			Template: MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "",
					GenerateName:    "",
					Namespace:       "",
					SelfLink:        "",
					UID:             "",
					ResourceVersion: "",
					Generation:      0,
					CreationTimestamp: metav1.Time{
						Time: time.Time{},
					},
					DeletionTimestamp: &metav1.Time{
						Time: time.Time{},
					},
					DeletionGracePeriodSeconds: nil,
					Labels:                     nil,
					Annotations:                nil,
					OwnerReferences:            nil,
					Initializers: &metav1.Initializers{
						Pending: nil,
						Result: &metav1.Status{
							TypeMeta: metav1.TypeMeta{
								Kind:       "",
								APIVersion: "",
							},
							ListMeta: metav1.ListMeta{
								SelfLink:        "",
								ResourceVersion: "",
								Continue:        "",
							},
							Status:  "",
							Message: "",
							Reason:  "",
							Details: &metav1.StatusDetails{
								Name:              "",
								Group:             "",
								Kind:              "",
								UID:               "",
								Causes:            nil,
								RetryAfterSeconds: 0,
							},
							Code: 0,
						},
					},
					Finalizers:    nil,
					ClusterName:   "",
					ManagedFields: nil,
				},
				Spec: MachineSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "",
						GenerateName:    "",
						Namespace:       "",
						SelfLink:        "",
						UID:             "",
						ResourceVersion: "",
						Generation:      0,
						CreationTimestamp: metav1.Time{
							Time: time.Time{},
						},
						DeletionTimestamp: &metav1.Time{
							Time: time.Time{},
						},
						DeletionGracePeriodSeconds: nil,
						Labels:                     nil,
						Annotations:                nil,
						OwnerReferences:            nil,
						Initializers: &metav1.Initializers{
							Pending: nil,
							Result: &metav1.Status{
								TypeMeta: metav1.TypeMeta{
									Kind:       "",
									APIVersion: "",
								},
								ListMeta: metav1.ListMeta{
									SelfLink:        "",
									ResourceVersion: "",
									Continue:        "",
								},
								Status:  "",
								Message: "",
								Reason:  "",
								Details: &metav1.StatusDetails{
									Name:              "",
									Group:             "",
									Kind:              "",
									UID:               "",
									Causes:            nil,
									RetryAfterSeconds: 0,
								},
								Code: 0,
							},
						},
						Finalizers:    nil,
						ClusterName:   "",
						ManagedFields: nil,
					},
					Taints: nil,
					ProviderSpec: ProviderSpec{
						Value: &runtime.RawExtension{
							Raw:    nil,
							Object: nil,
						},
					},
					ProviderID: nil,
				},
			},
			Strategy: &MachineDeploymentStrategy{
				RollingUpdate: &MachineRollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{
						Type:   0,
						IntVal: 0,
						StrVal: "",
					},
					MaxSurge: &intstr.IntOrString{
						Type:   0,
						IntVal: 0,
						StrVal: "",
					},
				},
			},
			MinReadySeconds:         nil,
			RevisionHistoryLimit:    nil,
			Paused:                  false,
			ProgressDeadlineSeconds: nil,
		},
		Status: MachineDeploymentStatus{
			ObservedGeneration:  0,
			Replicas:            0,
			UpdatedReplicas:     0,
			ReadyReplicas:       0,
			AvailableReplicas:   0,
			UnavailableReplicas: 0,
		},
	}

	replicas, found, err := unstructured.NestedInt64(u.Object, "spec", "replicas")
	if err == nil && found {
		machineDeployment.Spec.Replicas = pointer.Int32Ptr(int32(replicas))
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
			Name:            u.GetName(),
			GenerateName:    "",
			Namespace:       u.GetNamespace(),
			SelfLink:        "",
			UID:             u.GetUID(),
			ResourceVersion: "",
			Generation:      0,
			CreationTimestamp: metav1.Time{
				Time: time.Time{},
			},
			DeletionTimestamp: &metav1.Time{
				Time: time.Time{},
			},
			DeletionGracePeriodSeconds: nil,
			Labels:                     u.GetLabels(),
			Annotations:                u.GetAnnotations(),
			OwnerReferences:            u.GetOwnerReferences(),
			Initializers: &metav1.Initializers{
				Pending: nil,
				Result: &metav1.Status{
					TypeMeta: metav1.TypeMeta{
						Kind:       "",
						APIVersion: "",
					},
					ListMeta: metav1.ListMeta{
						SelfLink:        "",
						ResourceVersion: "",
						Continue:        "",
					},
					Status:  "",
					Message: "",
					Reason:  "",
					Details: &metav1.StatusDetails{
						Name:              "",
						Group:             "",
						Kind:              "",
						UID:               "",
						Causes:            nil,
						RetryAfterSeconds: 0,
					},
					Code: 0,
				},
			},
			Finalizers:    nil,
			ClusterName:   "",
			ManagedFields: nil,
		},
		Spec: MachineSetSpec{
			Replicas:        nil,
			MinReadySeconds: 0,
			Selector: metav1.LabelSelector{
				MatchLabels:      nil,
				MatchExpressions: nil,
			},
			Template: MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "",
					GenerateName:    "",
					Namespace:       "",
					SelfLink:        "",
					UID:             "",
					ResourceVersion: "",
					Generation:      0,
					CreationTimestamp: metav1.Time{
						Time: time.Time{},
					},
					DeletionTimestamp: &metav1.Time{
						Time: time.Time{},
					},
					DeletionGracePeriodSeconds: nil,
					Labels:                     nil,
					Annotations:                nil,
					OwnerReferences:            nil,
					Initializers: &metav1.Initializers{
						Pending: nil,
						Result: &metav1.Status{
							TypeMeta: metav1.TypeMeta{
								Kind:       "",
								APIVersion: "",
							},
							ListMeta: metav1.ListMeta{
								SelfLink:        "",
								ResourceVersion: "",
								Continue:        "",
							},
							Status:  "",
							Message: "",
							Reason:  "",
							Details: &metav1.StatusDetails{
								Name:              "",
								Group:             "",
								Kind:              "",
								UID:               "",
								Causes:            nil,
								RetryAfterSeconds: 0,
							},
							Code: 0,
						},
					},
					Finalizers:    nil,
					ClusterName:   "",
					ManagedFields: nil,
				},
				Spec: MachineSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "",
						GenerateName:    "",
						Namespace:       "",
						SelfLink:        "",
						UID:             "",
						ResourceVersion: "",
						Generation:      0,
						CreationTimestamp: metav1.Time{
							Time: time.Time{},
						},
						DeletionTimestamp: &metav1.Time{
							Time: time.Time{},
						},
						DeletionGracePeriodSeconds: nil,
						Labels:                     nil,
						Annotations:                nil,
						OwnerReferences:            nil,
						Initializers: &metav1.Initializers{
							Pending: nil,
							Result: &metav1.Status{
								TypeMeta: metav1.TypeMeta{
									Kind:       "",
									APIVersion: "",
								},
								ListMeta: metav1.ListMeta{
									SelfLink:        "",
									ResourceVersion: "",
									Continue:        "",
								},
								Status:  "",
								Message: "",
								Reason:  "",
								Details: &metav1.StatusDetails{
									Name:              "",
									Group:             "",
									Kind:              "",
									UID:               "",
									Causes:            nil,
									RetryAfterSeconds: 0,
								},
								Code: 0,
							},
						},
						Finalizers:    nil,
						ClusterName:   "",
						ManagedFields: nil,
					},
					Taints: nil,
					ProviderSpec: ProviderSpec{
						Value: &runtime.RawExtension{
							Raw:    nil,
							Object: nil,
						},
					},
					ProviderID: nil,
				},
			},
		},
		Status: MachineSetStatus{
			Replicas:             0,
			FullyLabeledReplicas: 0,
			ReadyReplicas:        0,
			AvailableReplicas:    0,
			ObservedGeneration:   0,
			ErrorMessage:         nil,
		},
	}

	replicas, found, err := unstructured.NestedInt64(u.Object, "spec", "replicas")
	if err == nil && found {
		machineSet.Spec.Replicas = pointer.Int32Ptr(int32(replicas))
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
			Name:            u.GetName(),
			Namespace:       u.GetNamespace(),
			UID:             u.GetUID(),
			Labels:          u.GetLabels(),
			Annotations:     u.GetAnnotations(),
			OwnerReferences: u.GetOwnerReferences(),
			ClusterName:     u.GetClusterName(),
		},
		Spec: MachineSpec{
			ProviderID: nil,
		},
		Status: MachineStatus{
			NodeRef: &corev1.ObjectReference{},
		},
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

	if m.Spec.Replicas != nil {
		unstructured.SetNestedField(u.Object, int64(*m.Spec.Replicas), "spec", "replicas")
	}

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

	if m.Spec.Replicas != nil {
		unstructured.SetNestedField(u.Object, int64(*m.Spec.Replicas), "spec", "replicas")
	}

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

	return &u
}
