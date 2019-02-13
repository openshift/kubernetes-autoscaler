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

package openshiftmachineapi

import (
	"fmt"
	"path"
	"sort"
	"testing"

	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/utils/pointer"
)

const (
	machineAnnotationKey = "machine.openshift.io/machine"
)

type nodeGroupConstructorTestCase struct {
	description string
	annotations map[string]string
	errors      bool
	minSize     int
	maxSize     int
	replicas    int32
	name        string
	namespace   string
	id          string
	debug       string
}

type testNodeGroupResizeTestCase struct {
	description string
	delta       int
	expected    int
	initial     int
	maxSize     string
	minSize     string
	errors      bool
}

func int32ptr(v int32) *int32 {
	return &v
}

func testNewNodeGroupProperties(t *testing.T, ng *nodegroup, tc nodeGroupConstructorTestCase) {
	t.Helper()

	if ng.Name() != tc.name {
		t.Errorf("expected %q, got %q", tc.name, ng.Name())
	}

	if ng.Namespace() != tc.namespace {
		t.Errorf("expected %q, got %q", tc.namespace, ng.Namespace())
	}

	if ng.MinSize() != tc.minSize {
		t.Errorf("expected %v, got %v", tc.minSize, ng.MinSize())
	}

	if ng.MaxSize() != tc.maxSize {
		t.Errorf("expected %v, got %v", tc.maxSize, ng.MaxSize())
	}

	if ng.Id() != tc.id {
		t.Errorf("expected %q, got %q", tc.id, ng.Id())
	}

	if ng.Debug() != tc.debug {
		t.Errorf("expected %q, got %q", tc.debug, ng.Debug())
	}

	if _, err := ng.TemplateNodeInfo(); err != cloudprovider.ErrNotImplemented {
		t.Error("expected error")
	}

	if expected, result := true, ng.Exist(); expected != result {
		t.Errorf("expected %t, got %t", expected, result)
	}

	if _, err := ng.Create(); err != cloudprovider.ErrAlreadyExist {
		t.Error("expected error")
	}

	if err := ng.Delete(); err != cloudprovider.ErrNotImplemented {
		t.Error("expected error")
	}

	if result := ng.Autoprovisioned(); result {
		t.Errorf("expected %t, got %t", false, result)
	}

	if _, err := ng.Nodes(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result, _ := ng.Nodes(); len(result) != 0 {
		t.Errorf("expected 0 nodes, got %v", len(result))
	}
}

func testNewMachineSetHelper(t *testing.T, testnum int, tc nodeGroupConstructorTestCase) {
	t.Helper()

	controller, stop := mustCreateTestController(t, testControllerConfig{})
	defer stop()

	tc.name = fmt.Sprintf("%d", testnum)
	tc.namespace = t.Name()
	tc.id = path.Join(tc.namespace, tc.name)
	tc.debug = fmt.Sprintf("%s (min: %d, max: %d, replicas: %d)", path.Join(tc.namespace, tc.name), tc.minSize, tc.maxSize, tc.replicas)

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:        tc.name,
			Namespace:   tc.namespace,
			Annotations: tc.annotations,
		},
		Spec: v1beta1.MachineSetSpec{
			Replicas: &tc.replicas,
		},
	}

	ng, err := newNodegroupFromMachineSet(controller, machineSet)

	if tc.errors && err == nil {
		t.Fatal("expected an error")
	}

	if !tc.errors && ng == nil {
		t.Fatalf("test case logic error: %v", err)
	}

	if !tc.errors {
		testNewNodeGroupProperties(t, ng, tc)
	}
}

func testNewMachineDeploymentHelper(t *testing.T, testnum int, tc nodeGroupConstructorTestCase) {
	t.Helper()

	controller, stop := mustCreateTestController(t, testControllerConfig{})
	defer stop()

	tc.name = fmt.Sprintf("%d", testnum)
	tc.namespace = t.Name()
	tc.id = path.Join(tc.namespace, tc.name)
	tc.debug = fmt.Sprintf("%s (min: %d, max: %d, replicas: %d)", path.Join(tc.namespace, tc.name), tc.minSize, tc.maxSize, tc.replicas)

	machineDeployment := &v1beta1.MachineDeployment{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:        tc.name,
			Namespace:   tc.namespace,
			Annotations: tc.annotations,
		},
		Spec: v1beta1.MachineDeploymentSpec{
			Replicas: &tc.replicas,
		},
	}

	ng, err := newNodegroupFromMachineDeployment(controller, machineDeployment)

	if tc.errors && err == nil {
		t.Fatal("expected an error")
	}

	if !tc.errors && ng == nil {
		t.Fatalf("test case logic error: %v", err)
	}

	if !tc.errors {
		testNewNodeGroupProperties(t, ng, tc)
	}
}

func TestNodeGroupNewNodeGroup(t *testing.T) {
	for i, tc := range []nodeGroupConstructorTestCase{{
		description: "errors because minSize is invalid",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "-1",
			nodeGroupMaxSizeAnnotationKey: "0",
		},
		errors: true,
	}, {
		description: "errors because maxSize is invalid",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "0",
			nodeGroupMaxSizeAnnotationKey: "-1",
		},
		errors: true,
	}, {
		description: "errors because minSize > maxSize",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "0",
		},
		errors: true,
	}, {
		description: "errors because maxSize < minSize",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "0",
		},
		errors: true,
	}, {
		description: "no error: min=0, max=0",
		minSize:     0,
		maxSize:     0,
		replicas:    0,
	}, {
		description: "no error: min=0, max=1",
		annotations: map[string]string{
			nodeGroupMaxSizeAnnotationKey: "1",
		},
		minSize:  0,
		maxSize:  1,
		replicas: 0,
	}, {
		description: "no error: min=1, max=10, replicas=5",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		minSize:  1,
		maxSize:  10,
		replicas: 5,
	}} {
		t.Logf("test #%d: %s", i, tc.description)

		testNewMachineSetHelper(t, i, tc)
		testNewMachineDeploymentHelper(t, i, tc)
	}
}

func testNodeGroupIncreaseSizeHelper(t *testing.T, ng *nodegroup, tc testNodeGroupResizeTestCase) {
	t.Helper()

	currReplicas, err := ng.TargetSize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if currReplicas != tc.initial {
		t.Errorf("expected %v, got %v", tc.initial, currReplicas)
	}

	err = ng.IncreaseSize(tc.delta)
	if tc.errors && err == nil {
		t.Fatal("expected an error")
	}

	if !tc.errors && err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testNodeGroupIncreaseMachineSet(t *testing.T, testnum int, tc testNodeGroupResizeTestCase) {
	t.Helper()

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				nodeGroupMinSizeAnnotationKey: tc.minSize,
				nodeGroupMaxSizeAnnotationKey: tc.maxSize,
			},
		},
		Spec: v1beta1.MachineSetSpec{
			Replicas: int32ptr(int32(tc.initial)),
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		machineObjects: []runtime.Object{
			machineSet,
		},
	})
	defer stop()

	ng, err := newNodegroupFromMachineSet(controller, machineSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testNodeGroupIncreaseSizeHelper(t, ng, tc)

	if !tc.errors {
		// A nodegroup is immutable; get a fresh copy.
		ms, err := ng.machineapiClient.MachineSets(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := pointer.Int32PtrDerefOr(ms.Spec.Replicas, 0)
		if int(actual) != tc.expected {
			t.Errorf("expected %v, got %v", tc.expected, actual)
		}
	}
}

func testNodeGroupIncreaseMachineDeployment(t *testing.T, testnum int, tc testNodeGroupResizeTestCase) {
	t.Helper()

	machineDeployment := &v1beta1.MachineDeployment{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machinedeployment",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				nodeGroupMinSizeAnnotationKey: tc.minSize,
				nodeGroupMaxSizeAnnotationKey: tc.maxSize,
			},
		},
		Spec: v1beta1.MachineDeploymentSpec{
			Replicas: int32ptr(int32(tc.initial)),
		},
	}

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: machineDeployment.Namespace,
			OwnerReferences: []v1.OwnerReference{{
				Kind: machineDeployment.Kind,
				Name: machineDeployment.Name,
			}},
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		machineObjects: []runtime.Object{
			machineDeployment,
			machineSet,
		},
	})
	defer stop()

	ng, err := newNodegroupFromMachineDeployment(controller, machineDeployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testNodeGroupIncreaseSizeHelper(t, ng, tc)

	if !tc.errors {
		// A nodegroup is immutable; get a fresh copy.
		md, err := ng.machineapiClient.MachineDeployments(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := pointer.Int32PtrDerefOr(md.Spec.Replicas, 0)
		if int(actual) != tc.expected {
			t.Errorf("expected %v, got %v", tc.expected, actual)
		}
	}
}

func testNodeGroupDecreaseSizeHelper(t *testing.T, ng *nodegroup, tc testNodeGroupResizeTestCase) {
	t.Helper()

	currReplicas, err := ng.TargetSize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if currReplicas != tc.initial {
		t.Errorf("expected %v, got %v", tc.initial, currReplicas)
	}

	err = ng.DecreaseTargetSize(tc.delta)
	if tc.errors && err == nil {
		t.Fatal("expected an error")
	}

	if !tc.errors && err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testNodeGroupDecreaseMachineSet(t *testing.T, testnum int, tc testNodeGroupResizeTestCase) {
	t.Helper()

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				nodeGroupMinSizeAnnotationKey: tc.minSize,
				nodeGroupMaxSizeAnnotationKey: tc.maxSize,
			},
		},
		Spec: v1beta1.MachineSetSpec{
			Replicas: int32ptr(int32(tc.initial)),
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		machineObjects: []runtime.Object{
			machineSet,
		},
	})
	defer stop()

	ng, err := newNodegroupFromMachineSet(controller, machineSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testNodeGroupDecreaseSizeHelper(t, ng, tc)

	if !tc.errors {
		// A nodegroup is immutable; get a fresh copy.
		ms, err := ng.machineapiClient.MachineSets(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := pointer.Int32PtrDerefOr(ms.Spec.Replicas, 0)
		if int(actual) != tc.expected {
			t.Errorf("expected %v, got %v", tc.expected, actual)
		}
	}
}

func testNodeGroupDecreaseMachineDeployment(t *testing.T, testnum int, tc testNodeGroupResizeTestCase) {
	t.Helper()

	machineDeployment := &v1beta1.MachineDeployment{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machinedeployment",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				nodeGroupMinSizeAnnotationKey: tc.minSize,
				nodeGroupMaxSizeAnnotationKey: tc.maxSize,
			},
		},
		Spec: v1beta1.MachineDeploymentSpec{
			Replicas: int32ptr(int32(tc.initial)),
		},
	}

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: machineDeployment.Namespace,
			OwnerReferences: []v1.OwnerReference{{
				Kind: machineDeployment.Kind,
				Name: machineDeployment.Name,
			}},
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		machineObjects: []runtime.Object{
			machineDeployment,
			machineSet,
		},
	})
	defer stop()

	ng, err := newNodegroupFromMachineDeployment(controller, machineDeployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testNodeGroupDecreaseSizeHelper(t, ng, tc)

	if !tc.errors {
		// A nodegroup is immutable; get a fresh copy.
		md, err := ng.machineapiClient.MachineDeployments(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := pointer.Int32PtrDerefOr(md.Spec.Replicas, 0)
		if int(actual) != tc.expected {
			t.Errorf("expected %v, got %v", tc.expected, actual)
		}
	}

}

func TestNodeGroupIncreaseSize(t *testing.T) {
	for i, tc := range []testNodeGroupResizeTestCase{{
		description: "errors because delta is negative",
		delta:       -1,
		errors:      true,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}, {
		description: "errors because initial+delta > maxSize",
		delta:       8,
		errors:      true,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}, {
		description: "no error as within bounds",
		delta:       1,
		expected:    4,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}} {
		t.Logf("test #%d: %q", i, tc.description)

		testNodeGroupIncreaseMachineSet(t, i, tc)
		testNodeGroupIncreaseMachineDeployment(t, i, tc)
	}
}

func TestNodeGroupDecreaseSize(t *testing.T) {
	for i, tc := range []testNodeGroupResizeTestCase{{
		description: "errors because delta is positive",
		delta:       1,
		errors:      true,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}, {
		description: "errors because delta exceeds node count",
		delta:       -4,
		errors:      true,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}, {
		description: "no error as within bounds",
		delta:       -1,
		expected:    2,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}} {
		t.Logf("test #%d: %s", i, tc.description)

		testNodeGroupDecreaseMachineSet(t, i, tc)
		testNodeGroupDecreaseMachineDeployment(t, i, tc)
	}
}

func TestNodeGroupMachineSetDeleteNodes(t *testing.T) {
	// Note: 10 is an upper bound for this test. Going beyond 10
	// will break the sorting that happens later in this function
	// because sort.Strings() will not do natural sorting and the
	// expected semantics in this test will fail.
	nodes := make([]*apiv1.Node, 10)
	machines := make([]*v1beta1.Machine, 10)
	nodeObjects := make([]runtime.Object, 10)
	machineObjects := make([]runtime.Object, 10)

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: "test-namespace",
			UID:       "abcdef12-a3d5-a45f-887b-6b49aa8fc218",
		},
		Spec: v1beta1.MachineSetSpec{
			Replicas: int32ptr(int32(len(machines))),
		},
	}

	for i := 0; i < len(nodes); i++ {
		nodes[i] = &apiv1.Node{
			TypeMeta: v1.TypeMeta{
				Kind: "Node",
			},
			ObjectMeta: v1.ObjectMeta{
				Name: fmt.Sprintf("node-%d", i),
				Annotations: map[string]string{
					machineAnnotationKey: fmt.Sprintf("test-namespace/machine-%d", i),
				},
			},
			Spec: apiv1.NodeSpec{
				ProviderID: fmt.Sprintf("providerid-%d", i),
			},
		}

		machines[i] = &v1beta1.Machine{
			TypeMeta: v1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("machine-%d", i),
				Namespace: "test-namespace",
				OwnerReferences: []v1.OwnerReference{{
					Name: machineSet.Name,
					Kind: machineSet.Kind,
					UID:  machineSet.UID,
				}},
			},
			Status: v1beta1.MachineStatus{
				NodeRef: &apiv1.ObjectReference{
					Kind: nodes[i].Kind,
					Name: nodes[i].Name,
				},
			},
		}

		nodeObjects[i] = nodes[i]
		machineObjects[i] = machines[i]
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		nodeObjects:    nodeObjects,
		machineObjects: append(machineObjects, machineSet),
	})
	defer stop()

	ng, err := newNodegroupFromMachineSet(controller, machineSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeNames, err := ng.Nodes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nodeNames) != len(nodes) {
		t.Fatalf("expected len=%v, got len=%v", len(nodes), len(nodeNames))
	}

	sort.Strings(nodeNames)

	for i := 0; i < len(nodes); i++ {
		if nodeNames[i] != nodes[i].Spec.ProviderID {
			t.Fatalf("expected %q, got %q", nodes[i].Spec.ProviderID, nodeNames[i])
		}
	}

	if err := ng.DeleteNodes(nodes[5:]); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	for i := 5; i < len(machines); i++ {
		key := fmt.Sprintf("machine-%d", i)
		machine, err := controller.clusterClientset.MachineV1beta1().Machines("test-namespace").Get(key, v1.GetOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, found := machine.Annotations[machineDeleteAnnotationKey]; !found {
			t.Errorf("expected annotation %q on machine %s", machineDeleteAnnotationKey, machine.Name)
		}
	}

	machineSet, err = controller.clusterClientset.MachineV1beta1().MachineSets(machineSet.Namespace).Get(machineSet.Name, v1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actual := pointer.Int32PtrDerefOr(machineSet.Spec.Replicas, 0); actual != 5 {
		t.Fatalf("expected 5 nodes, got %v", actual)
	}
}
