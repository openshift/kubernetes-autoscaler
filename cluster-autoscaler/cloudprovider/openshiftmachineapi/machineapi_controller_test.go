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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

type testControllerShutdownFunc func()

type testConfig struct {
	spec              *testSpec
	machineDeployment *v1beta1.MachineDeployment
	machineSet        *v1beta1.MachineSet
	machines          []*v1beta1.Machine
	nodes             []*apiv1.Node
}

type testSpec struct {
	annotations             map[string]string
	machineDeploymentName   string
	machineSetName          string
	namespace               string
	nodeCount               int
	replicaCount            int32
	rootIsMachineDeployment bool
}

func mustCreateTestController(t *testing.T, testConfigs ...*testConfig) (*machineController, testControllerShutdownFunc) {
	t.Helper()

	nodeObjects := make([]runtime.Object, 0)
	machineObjects := make([]runtime.Object, 0)

	for _, config := range testConfigs {
		for i := range config.nodes {
			nodeObjects = append(nodeObjects, config.nodes[i])
		}

		for i := range config.machines {
			machineObjects = append(machineObjects, config.machines[i])
		}

		machineObjects = append(machineObjects, config.machineSet)
		if config.machineDeployment != nil {
			machineObjects = append(machineObjects, config.machineDeployment)
		}
	}

	kubeclientSet := fakekube.NewSimpleClientset(nodeObjects...)
	clusterclientSet := fakeclusterapi.NewSimpleClientset(machineObjects...)
	controller, err := newMachineController(kubeclientSet, clusterclientSet, true)
	if err != nil {
		t.Fatal("failed to create test controller")
	}

	stopCh := make(chan struct{})
	if err := controller.run(stopCh); err != nil {
		t.Fatalf("failed to run controller: %v", err)
	}

	return controller, func() {
		close(stopCh)
	}
}

func mustCreateMachineSetTestConfig(namespace string, nodeCount int, replicaCount int32, annotations map[string]string) *testConfig {
	return mustCreateTestConfigs(mustCreateTestSpecs(namespace, 1, nodeCount, replicaCount, false, annotations)...)[0]
}

func mustCreateMachineSetTestConfigs(namespace string, configCount, nodeCount int, replicaCount int32, annotations map[string]string) []*testConfig {
	return mustCreateTestConfigs(mustCreateTestSpecs(namespace, configCount, nodeCount, replicaCount, false, annotations)...)
}

func mustCreateMachineDeploymentTestConfig(namespace string, nodeCount int, replicaCount int32, annotations map[string]string) *testConfig {
	return mustCreateTestConfigs(mustCreateTestSpecs(namespace, 1, nodeCount, replicaCount, true, annotations)...)[0]
}

func mustCreateMachineDeploymentTestConfigs(namespace string, configCount, nodeCount int, replicaCount int32, annotations map[string]string) []*testConfig {
	return mustCreateTestConfigs(mustCreateTestSpecs(namespace, configCount, nodeCount, replicaCount, true, annotations)...)
}

func mustCreateTestSpecs(namespace string, scalableResourceCount, nodeCount int, replicaCount int32, isMachineDeployment bool, annotations map[string]string) []testSpec {
	var specs []testSpec

	for i := 0; i < scalableResourceCount; i++ {
		specs = append(specs, testSpec{
			annotations:             annotations,
			machineDeploymentName:   fmt.Sprintf("machinedeployment-%d", i),
			machineSetName:          fmt.Sprintf("machineset-%d", i),
			namespace:               strings.ToLower(namespace),
			nodeCount:               nodeCount,
			replicaCount:            replicaCount,
			rootIsMachineDeployment: isMachineDeployment,
		})
	}

	return specs
}

func mustCreateTestConfigs(specs ...testSpec) []*testConfig {
	var result []*testConfig

	for i, spec := range specs {
		config := &testConfig{
			spec:     &specs[i],
			nodes:    make([]*apiv1.Node, spec.nodeCount),
			machines: make([]*v1beta1.Machine, spec.nodeCount),
		}

		config.machineSet = &v1beta1.MachineSet{
			TypeMeta: v1.TypeMeta{
				Kind: "MachineSet",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      spec.machineSetName,
				Namespace: spec.namespace,
				UID:       types.UID(spec.machineSetName),
			},
		}

		if !spec.rootIsMachineDeployment {
			config.machineSet.ObjectMeta.Annotations = spec.annotations
			config.machineSet.Spec.Replicas = int32ptr(spec.replicaCount)
		} else {
			config.machineDeployment = &v1beta1.MachineDeployment{
				TypeMeta: v1.TypeMeta{
					Kind: "MachineDeployment",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:        spec.machineDeploymentName,
					Namespace:   spec.namespace,
					UID:         types.UID(spec.machineDeploymentName),
					Annotations: spec.annotations,
				},
				Spec: v1beta1.MachineDeploymentSpec{
					Replicas: int32ptr(spec.replicaCount),
				},
			}

			config.machineSet.OwnerReferences = make([]v1.OwnerReference, 1)
			config.machineSet.OwnerReferences[0] = v1.OwnerReference{
				Name: config.machineDeployment.Name,
				Kind: config.machineDeployment.Kind,
				UID:  config.machineDeployment.UID,
			}
		}

		machineOwner := v1.OwnerReference{
			Name: config.machineSet.Name,
			Kind: config.machineSet.Kind,
			UID:  config.machineSet.UID,
		}

		for j := 0; j < spec.nodeCount; j++ {
			config.nodes[j], config.machines[j] = makeLinkedNodeAndMachine(j, spec.namespace, machineOwner)
		}

		result = append(result, config)
	}

	return result
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
			Name: fmt.Sprintf("%s-%s-node-%d", namespace, owner.Name, i),
			Annotations: map[string]string{
				machineAnnotationKey: fmt.Sprintf("%s/%s-%s-machine-%d", namespace, namespace, owner.Name, i),
			},
		},
		Spec: apiv1.NodeSpec{
			ProviderID: fmt.Sprintf("%s-%s-nodeid-%d", namespace, owner.Name, i),
		},
	}

	machine := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-machine-%d", namespace, owner.Name, i),
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

func int32ptr(v int32) *int32 {
	return &v
}

func addTestConfigs(t *testing.T, controller *machineController, testConfigs ...*testConfig) error {
	t.Helper()

	for _, config := range testConfigs {
		if config.machineDeployment != nil {
			if err := controller.machineDeploymentInformer.Informer().GetStore().Add(config.machineDeployment); err != nil {
				return err
			}
		}
		if err := controller.machineSetInformer.Informer().GetStore().Add(config.machineSet); err != nil {
			return err
		}
		for i := range config.machines {
			if err := controller.machineInformer.Informer().GetStore().Add(config.machines[i]); err != nil {
				return err
			}
		}
		for i := range config.nodes {
			if err := controller.nodeInformer.GetStore().Add(config.nodes[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteTestConfigs(t *testing.T, controller *machineController, testConfigs ...*testConfig) error {
	t.Helper()

	for _, config := range testConfigs {
		for i := range config.nodes {
			if err := controller.nodeInformer.GetStore().Delete(config.nodes[i]); err != nil {
				return err
			}
		}
		for i := range config.machines {
			if err := controller.machineInformer.Informer().GetStore().Delete(config.machines[i]); err != nil {
				return err
			}
		}
		if err := controller.machineSetInformer.Informer().GetStore().Delete(config.machineSet); err != nil {
			return err
		}
		if config.machineDeployment != nil {
			if err := controller.machineDeploymentInformer.Informer().GetStore().Delete(config.machineDeployment); err != nil {
				return err
			}
		}
	}
	return nil
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
	testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "10",
	})

	controller, stop := testObjs.newMachineController(t)
	defer stop()

	// Test #1: Verify known node can be found
	node, err := controller.findNodeByNodeName(testObjs.nodes[0].Name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node == nil {
		t.Fatal("expected lookup to be successful")
	}

	// Test #2: Verify non-existent node cannot be found
	node, err = controller.findNodeByNodeName(testObjs.nodes[0].Name + "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node != nil {
		t.Fatal("expected lookup to fail")
	}
}

func TestControllerMachinesInMachineSet(t *testing.T) {
	testObjs1 := newMachineSetTestObjs("testObjs1", 0, 5, 5, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "10",
	})

	controller, stop := testObjs1.newMachineController(t)
	defer stop()

	// Construct a second set of objects and add the machines,
	// nodes and the additional machineset to the existing set of
	// test objects in the controller. This gives us two
	// machinesets, each with their own machines and linked nodes.
	testObjs2 := newMachineSetTestObjs("testObjs2", 1, 5, 5, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "10",
	})

	for _, node := range testObjs2.nodes {
		if err := controller.nodeInformer.GetStore().Add(node); err != nil {
			t.Fatalf("error adding node, got %v", err)
		}
	}

	for _, machine := range testObjs2.machines {
		if err := controller.machineInformer.Informer().GetStore().Add(machine); err != nil {
			t.Fatalf("error adding machine, got %v", err)
		}
	}

	if err := controller.machineSetInformer.Informer().GetStore().Add(testObjs2.machineSet); err != nil {
		t.Fatalf("error adding machineset, got %v", err)
	}

	machinesInTestObjs1, err := controller.machineInformer.Lister().Machines(testObjs1.spec.namespace).List(labels.Everything())
	if err != nil {
		t.Fatalf("error listing machines: %v", err)
	}

	machinesInTestObjs2, err := controller.machineInformer.Lister().Machines(testObjs2.spec.namespace).List(labels.Everything())
	if err != nil {
		t.Fatalf("error listing machines: %v", err)
	}

	actual := len(machinesInTestObjs1) + len(machinesInTestObjs2)
	expected := len(testObjs1.machines) + len(testObjs2.machines)
	if actual != expected {
		t.Fatalf("expected %d machines, got %d", expected, actual)
	}

	// Sort results as order is not guaranteed.
	sort.Slice(machinesInTestObjs1, func(i, j int) bool {
		return machinesInTestObjs1[i].Name < machinesInTestObjs1[j].Name
	})
	sort.Slice(machinesInTestObjs2, func(i, j int) bool {
		return machinesInTestObjs2[i].Name < machinesInTestObjs2[j].Name
	})

	for i, m := range machinesInTestObjs1 {
		if m.Name != testObjs1.machines[i].Name {
			t.Errorf("expected %q, got %q", testObjs1.machines[i].Name, m.Name)
		}
		if m.Namespace != testObjs1.machines[i].Namespace {
			t.Errorf("expected %q, got %q", testObjs1.machines[i].Namespace, m.Namespace)
		}
	}

	for i, m := range machinesInTestObjs2 {
		if m.Name != testObjs2.machines[i].Name {
			t.Errorf("expected %q, got %q", testObjs2.machines[i].Name, m.Name)
		}
		if m.Namespace != testObjs2.machines[i].Namespace {
			t.Errorf("expected %q, got %q", testObjs2.machines[i].Namespace, m.Namespace)
		}
	}

	// Finally everything in the respective objects should be equal.
	if !reflect.DeepEqual(testObjs1.machines, machinesInTestObjs1) {
		t.Fatalf("expected %+v, got %+v", testObjs1.machines, machinesInTestObjs1)
	}
	if !reflect.DeepEqual(testObjs2.machines, machinesInTestObjs2) {
		t.Fatalf("expected %+v, got %+v", testObjs2.machines, machinesInTestObjs2)
	}
}

func TestControllerLookupNodeGroupForNonExistentNode(t *testing.T) {
	testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "10",
	})

	controller, stop := testObjs.newMachineController(t)
	defer stop()

	node := testObjs.nodes[0].DeepCopy()
	node.Spec.ProviderID = "does-not-exist"

	ng, err := controller.nodeGroupForNode(node)

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
	test := func(t *testing.T, testObjs *clusterTestConfig) {
		controller, stop := testObjs.newMachineController(t)
		defer stop()

		machine := testObjs.machines[0].DeepCopy()
		machine.OwnerReferences = []v1.OwnerReference{}
		if err := controller.machineInformer.Informer().GetStore().Update(machine); err != nil {
			t.Fatalf("unexpected error updating machine, got %v", err)
		}

		ng, err := controller.nodeGroupForNode(testObjs.nodes[0])
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ng != nil {
			t.Fatalf("unexpected nodegroup: %v", ng)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		})
		test(t, testObjs)
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		testObjs := newMachineDeploymentTestObjs(t.Name(), 0, 1, 1, map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		})
		test(t, testObjs)
	})
}

func TestControllerNodeGroupForNodeWithPositiveScalingBounds(t *testing.T) {
	test := func(t *testing.T, testObjs *clusterTestConfig) {
		controller, stop := testObjs.newMachineController(t)
		defer stop()

		ng, err := controller.nodeGroupForNode(testObjs.nodes[0])
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// We don't scale from 0 so nodes must belong to a
		// nodegroup that has a scale size of at least 1.
		if ng != nil {
			t.Fatalf("unexpected nodegroup: %v", ng)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		testObjs := newMachineSetTestObjs(t.Name(), 0, 1, 1, map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "1",
		})
		test(t, testObjs)
	})

	t.Run("MachineDeployment", func(t *testing.T) {
		testObjs := newMachineDeploymentTestObjs(t.Name(), 0, 1, 1, map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "1",
		})
		test(t, testObjs)
	})
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
