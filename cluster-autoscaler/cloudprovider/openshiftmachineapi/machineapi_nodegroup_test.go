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
	"strings"
	"testing"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/utils/pointer"
)

const (
	machineAnnotationKey = "machine.openshift.io/machine"
)

func TestNodeGroupNewNodeGroup(t *testing.T) {
	type testCase struct {
		description string
		annotations map[string]string
		errors      bool
		replicas    int32
		minSize     int
		maxSize     int
		name        string
		namespace   string
		id          string
		debug       string
		nodeCount   int
	}

	var testCases = []testCase{{
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
		errors:      false,
	}, {
		description: "no error: min=0, max=1",
		annotations: map[string]string{
			nodeGroupMaxSizeAnnotationKey: "1",
		},
		minSize:  0,
		maxSize:  1,
		replicas: 0,
		errors:   false,
	}, {
		description: "no error: min=1, max=10, replicas=5",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		minSize:   1,
		maxSize:   10,
		replicas:  5,
		nodeCount: 5,
		errors:    false,
	}}

	testNodeGroupProperties := func(t *testing.T, tc testCase, clusterObjs *clusterTestConfig) {
		controller, stop := clusterObjs.newMachineController(t)
		defer stop()

		ng, err := clusterObjs.newNodeGroup(t, controller)

		if tc.errors && err == nil {
			t.Fatal("expected an error")
		}

		if !tc.errors && ng == nil {
			t.Fatalf("test case logic error: %v", err)
		}

		if tc.errors {
			// if the test case is expected to error then
			// don't assert the remainder
			return
		}

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

		nodes, err := ng.Nodes()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(nodes) != tc.nodeCount {
			t.Errorf("expected %d nodes, got %v", tc.nodeCount, len(nodes))
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		for i, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				testObjs := newMachineSetTestObjs(t.Name(), i, tc.nodeCount, tc.replicas, tc.annotations)
				tc.namespace = testObjs.spec.namespace
				tc.name = fmt.Sprintf("%s%d", testObjs.spec.machineSetPrefix, i)
				tc.id = path.Join(tc.namespace, tc.name)
				tc.debug = fmt.Sprintf("%s (min: %d, max: %d, replicas: %d)", path.Join(tc.namespace, tc.name), tc.minSize, tc.maxSize, tc.replicas)
				testNodeGroupProperties(t, tc, testObjs)
			})
		}
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		for i, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				testObjs := newMachineDeploymentTestObjs(t.Name(), i, tc.nodeCount, tc.replicas, tc.annotations)
				tc.namespace = testObjs.spec.namespace
				tc.name = fmt.Sprintf("%s%d", testObjs.spec.machineDeploymentPrefix, i)
				tc.id = path.Join(tc.namespace, tc.name)
				tc.debug = fmt.Sprintf("%s (min: %d, max: %d, replicas: %d)", path.Join(tc.namespace, tc.name), tc.minSize, tc.maxSize, tc.replicas)
				testNodeGroupProperties(t, tc, testObjs)
			})
		}
	})
}

func TestNodeGroupIncreaseSize(t *testing.T) {
	type testCase struct {
		description string
		delta       int
		expected    int32
		initial     int32
		maxSize     string
		minSize     string
		errors      bool
	}

	testCases := []testCase{{
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
		errors:      false,
		expected:    4,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}}

	test := func(t *testing.T, tc *testCase, testObjs *clusterTestConfig) {
		controller, stop := testObjs.newMachineController(t)
		defer stop()

		ng, err := testObjs.newNodeGroup(t, controller)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		currReplicas, err := ng.TargetSize()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if currReplicas != int(tc.initial) {
			t.Errorf("expected %v, got %v", tc.initial, currReplicas)
		}

		err = ng.IncreaseSize(tc.delta)
		if tc.errors && err == nil {
			t.Fatal("expected an error")
		}

		if !tc.errors && err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tc.errors {
			return // expected to error
		}

		switch v := (ng.scalableResource).(type) {
		case *machineSetScalableResource:
			// A nodegroup is immutable; get a fresh copy.
			ms, err := ng.machineapiClient.MachineSets(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			actual := pointer.Int32PtrDerefOr(ms.Spec.Replicas, 0)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		case *machineDeploymentScalableResource:
			// A nodegroup is immutable; get a fresh copy.
			md, err := ng.machineapiClient.MachineDeployments(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			actual := pointer.Int32PtrDerefOr(md.Spec.Replicas, 0)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		default:
			t.Errorf("unexpected type: %T", v)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		for i, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				annotations := map[string]string{
					nodeGroupMinSizeAnnotationKey: tc.minSize,
					nodeGroupMaxSizeAnnotationKey: tc.maxSize,
				}
				test(t, &tc, newMachineSetTestObjs(t.Name(), i, int(tc.initial), tc.initial, annotations))
			})
		}
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		for i, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				annotations := map[string]string{
					nodeGroupMinSizeAnnotationKey: tc.minSize,
					nodeGroupMaxSizeAnnotationKey: tc.maxSize,
				}
				test(t, &tc, newMachineDeploymentTestObjs(t.Name(), i, int(tc.initial), tc.initial, annotations))
			})
		}
	})
}

func TestNodeGroupDecreaseSize(t *testing.T) {
	type testCase struct {
		description string
		delta       int
		expected    int
		initial     int
		maxSize     string
		minSize     string
		errors      bool
	}

	testCases := []testCase{{
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
		description: "errors because size+delta >= len(nodes)",
		delta:       -1,
		errors:      true,
		expected:    2,
		initial:     3,
		maxSize:     "10",
		minSize:     "1",
	}}

	test := func(t *testing.T, tc *testCase, testConfig *clusterTestConfig) {
		controller, stop := testConfig.newMachineController(t)
		defer stop()

		ng, err := testConfig.newNodeGroup(t, controller)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

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

		if tc.errors {
			return // expected to error
		}

		switch v := (ng.scalableResource).(type) {
		case *machineSetScalableResource:
			// A nodegroup is immutable; get a fresh copy.
			ms, err := ng.machineapiClient.MachineSets(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			actual := pointer.Int32PtrDerefOr(ms.Spec.Replicas, 0) + 1
			if int(actual) != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		case *machineDeploymentScalableResource:
			// A nodegroup is immutable; get a fresh copy.
			md, err := ng.machineapiClient.MachineDeployments(ng.Namespace()).Get(ng.Name(), v1.GetOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			actual := pointer.Int32PtrDerefOr(md.Spec.Replicas, 0)
			if int(actual) != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		default:
			t.Errorf("unexpected type: %T", v)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		for i, tc := range testCases {
			tc := tc
			t.Run(tc.description, func(t *testing.T) {
				annotations := map[string]string{
					nodeGroupMinSizeAnnotationKey: tc.minSize,
					nodeGroupMaxSizeAnnotationKey: tc.maxSize,
				}
				test(t, &tc, newMachineSetTestObjs(t.Name(), i, tc.initial, int32(tc.initial), annotations))
			})
		}
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		for i, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				annotations := map[string]string{
					nodeGroupMinSizeAnnotationKey: tc.minSize,
					nodeGroupMaxSizeAnnotationKey: tc.maxSize,
				}
				test(t, &tc, newMachineDeploymentTestObjs(t.Name(), i, tc.initial, int32(tc.initial), annotations))
			})
		}
	})
}

func TestNodeGroupDeleteNodes(t *testing.T) {
	test := func(t *testing.T, testObjs *clusterTestConfig) {
		controller, stop := testObjs.newMachineController(t)
		defer stop()

		ng, err := testObjs.newNodeGroup(t, controller)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		nodeNames, err := ng.Nodes()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(nodeNames) != len(testObjs.nodes) {
			t.Fatalf("expected len=%v, got len=%v", len(testObjs.nodes), len(nodeNames))
		}

		sort.Strings(nodeNames)

		for i := 0; i < len(nodeNames); i++ {
			if nodeNames[i] != testObjs.nodes[i].Spec.ProviderID {
				t.Fatalf("expected %q, got %q", testObjs.nodes[i].Spec.ProviderID, nodeNames[i])
			}
		}

		if err := ng.DeleteNodes(testObjs.nodes[5:]); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		for i := 5; i < len(testObjs.machines); i++ {
			machine, err := controller.clusterClientset.MachineV1beta1().Machines(testObjs.machines[i].Namespace).Get(testObjs.machines[i].Name, v1.GetOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, found := machine.Annotations[machineDeleteAnnotationKey]; !found {
				t.Errorf("expected annotation %q on machine %s", machineDeleteAnnotationKey, machine.Name)
			}
		}

		switch v := (ng.scalableResource).(type) {
		case *machineSetScalableResource:
			updatedMachineSet, err := controller.clusterClientset.MachineV1beta1().MachineSets(testObjs.machineSet.Namespace).Get(testObjs.machineSet.Name, v1.GetOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actual := pointer.Int32PtrDerefOr(updatedMachineSet.Spec.Replicas, 0); actual != 5 {
				t.Fatalf("expected 5 nodes, got %v", actual)
			}
		case *machineDeploymentScalableResource:
			updatedMachineDeployment, err := controller.clusterClientset.MachineV1beta1().MachineDeployments(testObjs.machineDeployment.Namespace).Get(testObjs.machineDeployment.Name, v1.GetOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actual := pointer.Int32PtrDerefOr(updatedMachineDeployment.Spec.Replicas, 0); actual != 5 {
				t.Fatalf("expected 5 nodes, got %v", actual)
			}
		default:
			t.Errorf("unexpected type: %T", v)
		}
	}

	// Note: 10 is an upper bound for the number of nodes/replicas
	// Going beyond 10 will break the sorting that happens in the
	// test() function because sort.Strings() will not do natural
	// sorting and the expected semantics in test() will fail.

	t.Run("MachineSet", func(t *testing.T) {
		test(t, newMachineSetTestObjs(t.Name(), 0, 10, 10, map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}))
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		test(t, newMachineDeploymentTestObjs(t.Name(), 0, 10, 10, map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}))
	})
}

func TestNodeGroupMachineSetDeleteNodesWithMismatchedNodes(t *testing.T) {
	nreplicas := 1

	machineSet0 := makeMachineSet(0, nreplicas, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "3",
	})

	machineSet1 := makeMachineSet(1, nreplicas, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "3",
	})

	node0, machine0 := makeLinkedNodeAndMachine(0, machineSet0.Namespace, v1.OwnerReference{
		Name: machineSet0.Name,
		Kind: machineSet0.Kind,
		UID:  machineSet0.UID,
	})

	node1, machine1 := makeLinkedNodeAndMachine(1, machineSet1.Namespace, v1.OwnerReference{
		Name: machineSet1.Name,
		Kind: machineSet1.Kind,
		UID:  machineSet1.UID,
	})

	controller, stop := mustCreateTestController(t, testControllerConfig{
		nodeObjects: []runtime.Object{
			node0,
			node1,
		},
		machineObjects: append([]runtime.Object{},
			machine0, machineSet0,
			machine1, machineSet1),
	})
	defer stop()

	ng0, err := newNodegroupFromMachineSet(controller, machineSet0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ng1, err := newNodegroupFromMachineSet(controller, machineSet1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deleting nodes that are not in ng0 should fail.
	err0 := ng0.DeleteNodes([]*apiv1.Node{node1})
	if err0 == nil {
		t.Error("expected an error")
	}
	expectedErr0 := `node "nodeid-1" doesn't belong to node group "test-namespace/machineset-0"`
	if !strings.Contains(err0.Error(), expectedErr0) {
		t.Errorf("expected: %q, got: %q", expectedErr0, err0.Error())
	}

	// Deleting nodes that are not in ng1 should fail.
	err1 := ng1.DeleteNodes([]*apiv1.Node{node0})
	if err1 == nil {
		t.Error("expected an error")
	}
	expectedErr1 := `node "nodeid-0" doesn't belong to node group "test-namespace/machineset-1"`
	if !strings.Contains(err1.Error(), expectedErr1) {
		t.Errorf("expected: %q, got: %q", expectedErr1, err1.Error())
	}

	// Deleting from correct node group should fail because
	// replicas would become <= 0.
	if err := ng0.DeleteNodes([]*apiv1.Node{node0}); err == nil {
		t.Error("expected error")
	}

	// Deleting from correct node group should fail because
	// replicas would become <= 0.
	if err := ng1.DeleteNodes([]*apiv1.Node{node1}); err == nil {
		t.Error("expected error")
	}
}
