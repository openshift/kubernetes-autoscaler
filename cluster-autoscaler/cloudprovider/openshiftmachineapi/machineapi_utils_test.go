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

package openshiftmachineapi_test

import (
	"strings"
	"testing"

	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/openshiftmachineapi"
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
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
	}, {
		description: "missing max annotation defaults to 0 and no error",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "0",
		},
	}, {
		description: "invalid min errors",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "-1",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: openshiftmachineapi.ErrInvalidMinAnnotation,
	}, {
		description: "invalid min errors",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "not-an-int",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: openshiftmachineapi.ErrInvalidMinAnnotation,
	}, {
		description: "invalid max errors",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "0",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "-1",
		},
		error: openshiftmachineapi.ErrInvalidMaxAnnotation,
	}, {
		description: "invalid max errors",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "0",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "not-an-int",
		},
		error: openshiftmachineapi.ErrInvalidMaxAnnotation,
	}, {
		description: "negative min errors",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "-1",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: openshiftmachineapi.ErrInvalidMinAnnotation,
	}, {
		description: "negative max errors",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "0",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "-1",
		},
		error: openshiftmachineapi.ErrInvalidMaxAnnotation,
	}, {
		description: "max < min errors",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "1",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		error: openshiftmachineapi.ErrInvalidMaxAnnotation,
	}, {
		description: "result is: min 0, max 0",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "0",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "0",
		},
		min: 0,
		max: 0,
	}, {
		description: "result is min 0, max 1",
		annotations: map[string]string{
			openshiftmachineapi.NodeGroupMinSizeAnnotationKey: "0",
			openshiftmachineapi.NodeGroupMaxSizeAnnotationKey: "1",
		},
		min: 0,
		max: 1,
	}} {
		machineSet := v1beta1.MachineSet{
			ObjectMeta: v1.ObjectMeta{
				Annotations: tc.annotations,
			},
		}

		min, max, err := openshiftmachineapi.ParseMachineSetBounds(&machineSet)

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
		machine     v1beta1.Machine
		machineSet  v1beta1.MachineSet
		owned       bool
	}{{
		description: "not owned as no owner references",
		machine:     v1beta1.Machine{},
		machineSet:  v1beta1.MachineSet{},
		owned:       false,
	}, {
		description: "not owned as not the same Kind",
		machine: v1beta1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "Other",
				}},
			},
		},
		machineSet: v1beta1.MachineSet{},
		owned:      false,
	}, {
		description: "not owned because no OwnerReference.Name",
		machine: v1beta1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "MachineSet",
					UID:  "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
				}},
			},
		},
		machineSet: v1beta1.MachineSet{
			ObjectMeta: v1.ObjectMeta{
				UID: "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
			},
		},
		owned: false,
	}, {
		description: "not owned as UID values don't match",
		machine: v1beta1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "MachineSet",
					Name: "foo",
					UID:  "ec23ebb0-bc60-443f-d139-046ec5046283",
				}},
			},
		},
		machineSet: v1beta1.MachineSet{
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
		machine: v1beta1.Machine{
			ObjectMeta: v1.ObjectMeta{
				OwnerReferences: []v1.OwnerReference{{
					Kind: "MachineSet",
					Name: "foo",
					UID:  "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
				}},
			},
		},
		machineSet: v1beta1.MachineSet{
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
		owned := openshiftmachineapi.MachineIsOwnedByMachineSet(&tc.machine, &tc.machineSet)

		if tc.owned != owned {
			t.Errorf("test #%d: expected %t, got %t", i, tc.owned, owned)
		}
	}
}
