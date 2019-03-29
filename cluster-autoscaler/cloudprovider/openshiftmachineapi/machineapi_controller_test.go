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
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	fakeclusterapi "github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset/fake"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

type testControllerConfig struct {
	nodeObjects    []runtime.Object
	machineObjects []runtime.Object
}

type testControllerShutdownFunc func()

func mustCreateTestController(t *testing.T, config testControllerConfig) (*machineController, testControllerShutdownFunc) {
	t.Helper()

	kubeclientSet := fakekube.NewSimpleClientset(config.nodeObjects...)
	clusterclientSet := fakeclusterapi.NewSimpleClientset(config.machineObjects...)

	controller, err := newMachineController(kubeclientSet, clusterclientSet, true)
	if err != nil {
		t.Fatalf("failed to create test controller")
	}

	stopCh := make(chan struct{})

	if err := controller.run(stopCh); err != nil {
		t.Fatalf("failed to run controller: %v", err)
	}

	return controller, func() {
		close(stopCh)
	}
}

type clusterTestConfig struct {
	spec              *clusterTestSpec
	machineDeployment *v1beta1.MachineDeployment
	machineSet        *v1beta1.MachineSet
	machines          []*v1beta1.Machine
	nodes             []*apiv1.Node
}

type clusterTestSpec struct {
	annotations             map[string]string
	id                      int
	machineDeploymentPrefix string
	machineSetPrefix        string
	namespace               string
	nodeCount               int
	replicaCount            int32
	rootIsMachineDeployment bool
}

func (config clusterTestConfig) newNodeGroup(t *testing.T, c *machineController) (*nodegroup, error) {
	if config.machineDeployment != nil {
		return newNodegroupFromMachineDeployment(c, config.machineDeployment)
	}
	return newNodegroupFromMachineSet(c, config.machineSet)
}

func (config clusterTestConfig) newMachineController(t *testing.T) (*machineController, testControllerShutdownFunc) {
	nodeObjects := make([]runtime.Object, len(config.nodes))
	machineObjects := make([]runtime.Object, len(config.machines))

	for i := range config.nodes {
		nodeObjects[i] = config.nodes[i]
	}

	for i := range config.machines {
		machineObjects[i] = config.machines[i]
	}

	machineObjects = append(machineObjects, config.machineSet)
	if config.machineDeployment != nil {
		machineObjects = append(machineObjects, config.machineDeployment)
	}

	return mustCreateTestController(t, testControllerConfig{
		nodeObjects:    nodeObjects,
		machineObjects: machineObjects,
	})
}

func newMachineSetTestObjs(namespace string, id, nodeCount int, replicaCount int32, annotations map[string]string) *clusterTestConfig {
	spec := &clusterTestSpec{
		id:                      id,
		annotations:             annotations,
		machineSetPrefix:        fmt.Sprintf("machineset-%s-", namespace),
		namespace:               namespace,
		nodeCount:               nodeCount,
		replicaCount:            replicaCount,
		rootIsMachineDeployment: false,
	}

	return makeClusterObjs(spec)
}

func newMachineDeploymentTestObjs(namespace string, id, nodeCount int, replicaCount int32, annotations map[string]string) *clusterTestConfig {
	spec := &clusterTestSpec{
		id:                      id,
		annotations:             annotations,
		machineDeploymentPrefix: fmt.Sprintf("machinedeployment-%s-", namespace),
		machineSetPrefix:        fmt.Sprintf("machineset-%s-", namespace),
		namespace:               strings.ToLower(namespace),
		nodeCount:               nodeCount,
		replicaCount:            replicaCount,
		rootIsMachineDeployment: true,
	}

	return makeClusterObjs(spec)
}

func makeClusterObjs(spec *clusterTestSpec) *clusterTestConfig {
	objs := clusterTestConfig{
		spec:     spec,
		nodes:    make([]*apiv1.Node, spec.nodeCount),
		machines: make([]*v1beta1.Machine, spec.nodeCount),
	}

	objs.machineSet = &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("%s%d", spec.machineSetPrefix, spec.id),
			Namespace: spec.namespace,
			UID:       types.UID(fmt.Sprintf("%s%d", spec.machineSetPrefix, spec.id)),
		},
	}

	if !spec.rootIsMachineDeployment {
		objs.machineSet.ObjectMeta.Annotations = spec.annotations
		objs.machineSet.Spec.Replicas = int32ptr(spec.replicaCount)
	} else {
		objs.machineDeployment = &v1beta1.MachineDeployment{
			TypeMeta: v1.TypeMeta{
				Kind: "MachineDeployment",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:        fmt.Sprintf("%s%d", spec.machineDeploymentPrefix, spec.id),
				Namespace:   spec.namespace,
				UID:         types.UID(fmt.Sprintf("%s%d", spec.machineDeploymentPrefix, spec.id)),
				Annotations: spec.annotations,
			},
			Spec: v1beta1.MachineDeploymentSpec{
				Replicas: int32ptr(spec.replicaCount),
			},
		}

		objs.machineSet.OwnerReferences = make([]v1.OwnerReference, 1)
		objs.machineSet.OwnerReferences[0] = v1.OwnerReference{
			Name: objs.machineDeployment.Name,
			Kind: objs.machineDeployment.Kind,
			UID:  objs.machineDeployment.UID,
		}
	}

	machineOwner := v1.OwnerReference{
		Name: objs.machineSet.Name,
		Kind: objs.machineSet.Kind,
		UID:  objs.machineSet.UID,
	}

	for i := 0; i < spec.nodeCount; i++ {
		objs.nodes[i], objs.machines[i] = makeLinkedNodeAndMachine(i, spec.namespace, machineOwner)
	}

	return &objs
}

func int32ptr(v int32) *int32 {
	return &v
}

func makeMachineSet(i int, replicaCount int, annotations map[string]string) *v1beta1.MachineSet {
	return &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:        fmt.Sprintf("machineset-%d", i),
			Namespace:   "test-namespace",
			UID:         types.UID(fmt.Sprintf("machineset-%d", i)),
			Annotations: annotations,
		},
		Spec: v1beta1.MachineSetSpec{
			Replicas: int32ptr(int32(replicaCount)),
		},
	}
}

// makeLinkedNodeAndMachine creates a node and machine. The machine
// has its NodeRef set to the new node and the new machine's owner
// reference is set to owner.
func makeLinkedNodeAndMachine(i int, namespace string, owner v1.OwnerReference) (*apiv1.Node, *v1beta1.Machine) {
	node := &apiv1.Node{
		TypeMeta: v1.TypeMeta{
			Kind: "Node",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: fmt.Sprintf("node-%d", i),
			Annotations: map[string]string{
				machineAnnotationKey: fmt.Sprintf("%s/machine-%d", namespace, i),
			},
		},
		Spec: apiv1.NodeSpec{
			ProviderID: fmt.Sprintf("nodeid-%d", i),
		},
	}

	machine := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("machine-%d", i),
			Namespace: namespace,
			OwnerReferences: []v1.OwnerReference{{
				Name: owner.Name,
				Kind: owner.Kind,
				UID:  owner.UID,
			}},
		},
		Status: v1beta1.MachineStatus{
			NodeRef: &apiv1.ObjectReference{
				Kind: node.Kind,
				Name: node.Name,
			},
		},
	}

	return node, machine
}

func TestControllerFindMachineByID(t *testing.T) {
	type testCase struct {
		description    string
		name           string
		namespace      string
		lookupSucceeds bool
	}

	var testCases = []testCase{{
		description:    "lookup fails",
		lookupSucceeds: false,
		name:           "machine-does-not-exist",
		namespace:      "namespace-does-not-exist",
	}, {
		description:    "lookup fails in valid namespace",
		lookupSucceeds: false,
		name:           "machine-does-not-exist-in-existing-namespace",
	}, {
		description:    "lookup succeeds",
		lookupSucceeds: true,
	}}

	test := func(t *testing.T, tc testCase, testObjs *clusterTestConfig) {
		controller, stop := testObjs.newMachineController(t)
		defer stop()

		machine, err := controller.findMachine(path.Join(tc.namespace, tc.name))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tc.lookupSucceeds && machine == nil {
			t.Error("expected success, findMachine failed")
		}

		if tc.lookupSucceeds && machine != nil {
			if machine.Name != tc.name {
				t.Errorf("expected %q, got %q", tc.name, machine.Name)
			}
			if machine.Namespace != tc.namespace {
				t.Errorf("expected %q, got %q", tc.namespace, machine.Namespace)
			}
		}
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, map[string]string{
				nodeGroupMinSizeAnnotationKey: "1",
				nodeGroupMaxSizeAnnotationKey: "10",
			})
			if tc.name == "" {
				tc.name = testObjs.machines[0].Name
			}
			if tc.namespace == "" {
				tc.namespace = testObjs.machines[0].Namespace
			}
			test(t, tc, testObjs)
		})
	}
}

func TestControllerFindMachineOwner(t *testing.T) {
	testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "10",
	})

	controller, stop := testObjs.newMachineController(t)
	defer stop()

	// Test #1: Lookup succeeds
	testResult1, err := controller.findMachineOwner(testObjs.machines[0].DeepCopy())
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if testResult1 == nil {
		t.Fatal("expected non-nil result")
	}
	expected := fmt.Sprintf("%s%d", testObjs.spec.machineSetPrefix, 0)
	if expected != testResult1.Name {
		t.Errorf("expected %q, got %q", expected, testResult1.Name)
	}

	// Test #2: Lookup fails as the machine UUID != machineset UUID
	testMachine2 := testObjs.machines[0].DeepCopy()
	testMachine2.OwnerReferences[0].UID = "does-not-match-machineset"
	testResult2, err := controller.findMachineOwner(testMachine2)
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if testResult2 != nil {
		t.Fatal("expected nil result")
	}

	// Test #3: Delete the MachineSet and lookup should fail
	if err := controller.machineSetInformer.Informer().GetStore().Delete(testResult1); err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	testResult3, err := controller.findMachineOwner(testObjs.machines[0].DeepCopy())
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if testResult3 != nil {
		t.Fatal("expected lookup to fail")
	}
}

func TestControllerFindMachineByNodeProviderID(t *testing.T) {
	testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "10",
	})

	controller, stop := testObjs.newMachineController(t)
	defer stop()

	// Test #1: Verify node can be found because it has a
	// ProviderID value and a machine annotation.
	machine, err := controller.findMachineByNodeProviderID(testObjs.nodes[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if machine == nil {
		t.Fatal("expected to find machine")
	}
	if !reflect.DeepEqual(machine, testObjs.machines[0]) {
		t.Fatalf("expected machines to be equal - expected %+v, got %+v", testObjs.machines[0], machine)
	}

	// Test #2: Verify node is not found if it has a non-existent ProviderID
	node := testObjs.nodes[0].DeepCopy()
	node.Spec.ProviderID = ""
	nonExistentMachine, err := controller.findMachineByNodeProviderID(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nonExistentMachine != nil {
		t.Fatal("expected find to fail")
	}

	// Test #3: Verify node is not found if the stored object has
	// no "machine" annotation
	node = testObjs.nodes[0].DeepCopy()
	delete(node.Annotations, machineAnnotationKey)
	if err := controller.nodeInformer.GetStore().Update(node); err != nil {
		t.Fatalf("unexpected error updating node, got %v", err)
	}
	nonExistentMachine, err = controller.findMachineByNodeProviderID(testObjs.nodes[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nonExistentMachine != nil {
		t.Fatal("expected find to fail")
	}
}

func TestControllerFindNodeByNodeName(t *testing.T) {
	testNode := &apiv1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: "ip-10-0-18-236.us-east-2.compute.internal",
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		nodeObjects: []runtime.Object{
			testNode,
		},
	})
	defer stop()

	// Verify inserted node can be found
	node, err := controller.findNodeByNodeName("ip-10-0-18-236.us-east-2.compute.internal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node == nil {
		t.Fatal("expected a node")
	}

	// Verify node is identical to that added to the store
	if !reflect.DeepEqual(*node, *testNode) {
		t.Fatalf("expected %+v, got %+v", testNode, node)
	}

	// Verify that a successful findNodeByNodeName returns a DeepCopy().
	if node == testNode {
		t.Fatalf("expected a DeepCopy to be returned from findMachine()")
	}

	// Verify non-existent node doesn't error but is not found
	node, err = controller.findNodeByNodeName("does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node != nil {
		t.Fatalf("didn't expect to find a node")
	}
}

func TestControllerMachinesInMachineSet(t *testing.T) {
	testMachineSet1 := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineSet1",
			Namespace: "test-namespace",
			UID:       uuid1,
		},
	}

	testMachineSet2 := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineSet2",
			Namespace: "test-namespace",
			UID:       "a-value-that-is-not-uuid1-or-uuid2",
		},
	}

	objects := []runtime.Object{
		testMachineSet1,
		testMachineSet2,
	}

	testMachines := make([]*v1beta1.Machine, 10)

	for i := 0; i < 10; i++ {
		testMachines[i] = &v1beta1.Machine{
			TypeMeta: v1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("machine-%d", i),
				Namespace: "test-namespace",
			},
		}
		// Only even numbered machines belong to testMachineSet1
		if i%2 == 0 {
			testMachines[i].OwnerReferences = []v1.OwnerReference{{
				Kind: "MachineSet",
				UID:  uuid1,
				Name: "testMachineSet1",
			}}
		}
		objects = append(objects, testMachines[i])
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		machineObjects: objects,
	})
	defer stop()

	foundMachines, err := controller.machinesInMachineSet(testMachineSet1)
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if len(foundMachines) != 5 {
		t.Fatalf("expected 5 machines, got %v", len(foundMachines))
	}

	// Sort results as order is not guaranteed.
	sort.Slice(foundMachines, func(i, j int) bool {
		return foundMachines[i].Name < foundMachines[j].Name
	})

	for i := 0; i < len(foundMachines); i++ {
		if !reflect.DeepEqual(*testMachines[2*i], *foundMachines[i]) {
			t.Errorf("expected %s, got %s", testMachines[2*i].Name, foundMachines[i].Name)
		}
		// Verify that a successful result is a copy
		if testMachines[2*i] == foundMachines[i] {
			t.Errorf("expected a copy")
		}
	}
}

func TestControllerNodeGroupsSizes(t *testing.T) {
	for i, tc := range []struct {
		description string
		annotations map[string]string
		count       int
	}{{
		description: "errors because minSize is invalid",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "-1",
			nodeGroupMaxSizeAnnotationKey: "0",
		},
	}, {
		description: "errors because maxSize is invalid",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "0",
			nodeGroupMaxSizeAnnotationKey: "-1",
		},
	}, {
		description: "errors because minSize > maxSize",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "0",
		},
	}, {
		description: "errors because maxSize < minSize",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "0",
		},
	}, {
		description: "success, number of nodegroups == 1",
		annotations: map[string]string{
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		count: 1,
	}, {
		description: "success, number of nodegroups == 1",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		count: 1,
	}} {
		machineSet := &v1beta1.MachineSet{
			TypeMeta: v1.TypeMeta{
				Kind: "MachineSet",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:        fmt.Sprintf("machineset-%d", i),
				Namespace:   "test-namespace",
				Annotations: tc.annotations,
			},
		}

		controller, stop := mustCreateTestController(t, testControllerConfig{
			machineObjects: []runtime.Object{
				machineSet,
			},
		})
		defer stop()

		nodegroups, err := controller.nodeGroups()
		if tc.count == 0 && err == nil {
			t.Fatalf("expected an error")
		}

		if l := len(nodegroups); l != tc.count {
			t.Errorf("expected %v, got %v", tc.count, l)
		}
	}
}

func TestControllerLookupNodeGroupForNodeThatDoesNotExist(t *testing.T) {
	machine := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machine",
			Namespace: "test-namespace",
			OwnerReferences: []v1.OwnerReference{{
				Kind: "MachineSet",
				UID:  uuid1,
				Name: "testMachineSet",
			}},
		},
	}

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: "test-namespace",
			UID:       uuid1,
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		machineObjects: []runtime.Object{
			machine,
			machineSet,
		},
	})
	defer stop()

	ng, err := controller.nodeGroupForNode(&apiv1.Node{
		TypeMeta: v1.TypeMeta{
			Kind: "Node",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "node",
		},
	})

	// Looking up a node that doesn't exist doesn't generate an
	// error. But, equally, the ng should actually be nil.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ng != nil {
		t.Fatalf("unexpected nodegroup: %v", ng)
	}
}

func TestControllerNodeGroupForNodeWithMissingMachineOwner(t *testing.T) {
	node := &apiv1.Node{
		TypeMeta: v1.TypeMeta{
			Kind: "Node",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "node",
			Annotations: map[string]string{
				machineAnnotationKey: "test-namespace/machine",
			},
		},
		Spec: apiv1.NodeSpec{
			ProviderID: "provider-id",
		},
	}

	machine := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machine",
			Namespace: "test-namespace",
		},
	}

	machineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: "test-namespace",
			UID:       uuid1,
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		nodeObjects: []runtime.Object{
			node,
		},
		machineObjects: []runtime.Object{
			machine,
			machineSet,
		},
	})
	defer stop()

	ng, err := controller.nodeGroupForNode(&apiv1.Node{
		TypeMeta: v1.TypeMeta{
			Kind: "Node",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "node",
		},
		Spec: apiv1.NodeSpec{
			ProviderID: "provider-id",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ng != nil {
		t.Fatalf("unexpected nodegroup: %v", ng)
	}
}

func TestControllerNodeGroups(t *testing.T) {
	type testCase struct {
		description string
		annotations map[string]string
		errors      bool
		nodegroups  int
	}

	var testCases = []testCase{{
		description: "errors with bad annotations",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "-1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		nodegroups: 0,
		errors:     true,
	}, {
		description: "success with zero bounds",
		nodegroups:  0,
		errors:      false,
	}, {
		description: "success with positive bounds",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		nodegroups: 1,
		errors:     false,
	}}

	test := func(t *testing.T, tc testCase, testObjs *clusterTestConfig) {
		controller, stop := testObjs.newMachineController(t)
		defer stop()

		nodegroups, err := controller.nodeGroups()
		if tc.errors && err == nil {
			t.Errorf("expected an error")
		}

		if !tc.errors && err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if tc.errors && nodegroups != nil {
			t.Fatalf("test case logic error")
		}

		if actual := len(nodegroups); actual != tc.nodegroups {
			t.Errorf("expected %d, got %d", tc.nodegroups, actual)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, tc.annotations)
				test(t, tc, testObjs)
			})
		}
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				test(t, tc, newMachineDeploymentTestObjs(t.Name(), 0, 1, 1, tc.annotations))
			})
		}
	})
}

func TestControllerNodeGroupForNodeLookup(t *testing.T) {
	type testCase struct {
		description    string
		annotations    map[string]string
		lookupSucceeds bool
	}

	var testCases = []testCase{{
		description:    "lookup is nil because no annotations",
		annotations:    map[string]string{},
		lookupSucceeds: false,
	}, {
		description: "lookup is nil because scaling range == 0",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "1",
		},
		lookupSucceeds: false,
	}, {
		description: "lookup is successful because scaling range >= 1",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "2",
		},
		lookupSucceeds: true,
	}}

	test := func(t *testing.T, tc testCase, testObjs *clusterTestConfig, node *apiv1.Node) {
		controller, stop := testObjs.newMachineController(t)
		defer stop()

		ng, err := controller.nodeGroupForNode(node)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ng == nil && tc.lookupSucceeds {
			t.Fatalf("expected non-nil from lookup")
		}

		if ng != nil && !tc.lookupSucceeds {
			t.Fatalf("expected nil from lookup")
		}

		if !tc.lookupSucceeds {
			return
		}

		var expected string

		if testObjs.machineDeployment != nil {
			expected = path.Join(testObjs.machineDeployment.Namespace, testObjs.machineDeployment.Name)
		} else {
			expected = path.Join(testObjs.machineSet.Namespace, testObjs.machineSet.Name)
		}

		if actual := ng.Id(); actual != expected {
			t.Errorf("expected %q, got %q", expected, actual)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, tc.annotations)
				test(t, tc, testObjs, testObjs.nodes[0].DeepCopy())
			})
		}
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				testObjs := newMachineDeploymentTestObjs(t.Name(), 0, 1, 1, tc.annotations)
				test(t, tc, testObjs, testObjs.nodes[0].DeepCopy())
			})
		}
	})
}
