/*
Copyright 2019 The Kubernetes Authors.

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

package clusterapi_test

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/clusterapi"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestParseMachineSetBounds(t *testing.T) {
	for i, tc := range []struct {
		description string
		annotations map[string]string
		error       error
		min         int
		max         int
	}{{
		description: "missing min annotation defaults to 0 and no error",
		annotations: map[string]string{
			clusterapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
	}, {
		description: "missing max annotation defaults to 0 and no error",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "0",
		},
	}, {
		description: "invalid min errors",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "-1",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: clusterapi.ErrInvalidMinAnnotation,
	}, {
		description: "invalid min errors",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "not-an-int",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: clusterapi.ErrInvalidMinAnnotation,
	}, {
		description: "invalid max errors",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "0",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "-1",
		},
		error: clusterapi.ErrInvalidMaxAnnotation,
	}, {
		description: "invalid max errors",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "0",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "not-an-int",
		},
		error: clusterapi.ErrInvalidMaxAnnotation,
	}, {
		description: "negative min errors",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "-1",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: clusterapi.ErrInvalidMinAnnotation,
	}, {
		description: "negative max errors",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "0",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "-1",
		},
		error: clusterapi.ErrInvalidMaxAnnotation,
	}, {
		description: "max < min errors",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "1",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: clusterapi.ErrInvalidMaxAnnotation,
	}, {
		description: "result is: min 0, max 0",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "0",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		min: 0,
		max: 0,
	}, {
		description: "result is min 0, max 1",
		annotations: map[string]string{
			clusterapi.NodeGroupMinSizeAnnotationKey: "0",
			clusterapi.NodeGroupMaxSizeAnnotationKey: "1",
		},
		min: 0,
		max: 1,
	}} {
		machineSet := v1alpha1.MachineSet{
			ObjectMeta: v1.ObjectMeta{
				Annotations: tc.annotations,
			},
		}

		min, max, err := clusterapi.ParseMachineSetBounds(&machineSet)

		if tc.error != nil && err == nil {
			t.Fatalf("test #%d: expected an error", i)
		}

		if tc.error != nil && tc.error != err {
			if !strings.HasPrefix(err.Error(), tc.error.Error()) {
				t.Errorf("test #%d: expected message to have prefix %q, got %q",
					i, tc.error.Error(), err)
			}
		}

		if tc.error == nil {
			if tc.min != min {
				t.Errorf("test #%d: expected min %d, got %d", i, tc.min, min)
			}
			if tc.max != max {
				t.Errorf("test #%d: expected max %d, got %d", i, tc.max, max)
			}
		}
	}
}

func TestMachineIsOwnedByMachineSet(t *testing.T) {
	for i, tc := range []struct {
		description string
		machine     v1alpha1.Machine
		machineSet  v1alpha1.MachineSet
		owned       bool
	}{{
		description: "not owned as no owner references",
		machine:     v1alpha1.Machine{},
		machineSet:  v1alpha1.MachineSet{},
		owned:       false,
	}, {
		description: "not owned as not the same Kind",
		machine: v1alpha1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "Other",
				}},
			},
		},
		machineSet: v1alpha1.MachineSet{},
		owned:      false,
	}, {
		description: "not owned because no OwnerReference.Name",
		machine: v1alpha1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "MachineSet",
					UID:  "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
				}},
			},
		},
		machineSet: v1alpha1.MachineSet{
			ObjectMeta: v1.ObjectMeta{
				UID: "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
			},
		},
		owned: false,
	}, {
		description: "not owned as UID values don't match",
		machine: v1alpha1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "MachineSet",
					Name: "foo",
					UID:  "ec23ebb0-bc60-443f-d139-046ec5046283",
				}},
			},
		},
		machineSet: v1alpha1.MachineSet{
			TypeMeta: v1.TypeMeta{
				Kind: "MachineSet",
			},
			ObjectMeta: v1.ObjectMeta{
				UID: "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
			},
		},
		owned: false,
	}, {
		description: "owned as UID values match and same Kind and Name not empty",
		machine: v1alpha1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "MachineSet",
					Name: "foo",
					UID:  "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
				}},
			},
		},
		machineSet: v1alpha1.MachineSet{
			TypeMeta: v1.TypeMeta{
				Kind: "MachineSet",
			},
			ObjectMeta: v1.ObjectMeta{
				Name: "foo",
				UID:  "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
			},
		},
		owned: true,
	}} {
		owned := clusterapi.MachineIsOwnedByMachineSet(&tc.machine, &tc.machineSet)

		if tc.owned != owned {
			t.Errorf("test #%d: expected %t, got %t", i, tc.owned, owned)
		}
	}
}
