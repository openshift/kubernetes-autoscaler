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

func makeMachineOwner(i int, replicaCount int, annotations map[string]string, ownedByMachineDeployment bool) (*v1beta1.MachineSet, *v1beta1.MachineDeployment) {
	machineSet := v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("machineset-%d", i),
			Namespace: "test-namespace",
			UID:       types.UID(fmt.Sprintf("machineset-%d", i)),
		},
		Spec: v1beta1.MachineSetSpec{
			Replicas: int32ptr(int32(replicaCount)),
		},
	}

	if !ownedByMachineDeployment {
		machineSet.ObjectMeta.Annotations = annotations
		return &machineSet, nil
	}

	machineDeployment := v1beta1.MachineDeployment{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:        fmt.Sprintf("machinedeployment-%d", i),
			Namespace:   "test-namespace",
			UID:         types.UID(fmt.Sprintf("machinedeployment-%d", i)),
			Annotations: annotations,
		},
		Spec: v1beta1.MachineDeploymentSpec{
			Replicas: int32ptr(int32(replicaCount)),
		},
	}

	machineSet.OwnerReferences = append(machineSet.OwnerReferences, v1.OwnerReference{
		Name: machineDeployment.Name,
		Kind: machineDeployment.Kind,
		UID:  machineDeployment.UID,
	})

	return &machineSet, &machineDeployment
}

func TestControllerFindMachineByID(t *testing.T) {
	controller, stop := mustCreateTestController(t, testControllerConfig{})
	defer stop()

	testMachine := &v1beta1.Machine{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}

	// Verify machine count starts at 0.
	machines, err := controller.machineInformer.Lister().Machines(v1.NamespaceAll).List(labels.Everything())
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if len(machines) != 0 {
		t.Fatalf("expected 0 machines, got %d", len(machines))
	}

	controller.machineInformer.Informer().GetStore().Add(testMachine)

	// Verify machine count goes to 1.
	machines, err = controller.machineInformer.Lister().Machines(v1.NamespaceAll).List(labels.Everything())
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(machines))
	}

	// Verify inserted machine matches retrieved machine
	if !reflect.DeepEqual(*machines[0], *testMachine) {
		t.Fatalf("expected machines to be equal")
	}

	// Verify findMachine() can find the test machine
	foundMachine, err := controller.findMachine(path.Join(testMachine.Namespace, testMachine.Name))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if foundMachine == nil {
		t.Fatalf("expected to find machine %q in namespace %q", testMachine.Name, testMachine.Namespace)
	}

	// Verify that a successful findMachine returns a DeepCopy().
	if foundMachine == testMachine {
		t.Fatalf("expected a copy")
	}

	// Verify non-existent machine is not found by findMachine()
	foundMachine, err = controller.findMachine(path.Join("different-namespace", testMachine.Name))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if foundMachine != nil {
		t.Fatalf("expected findMachine() to return nil")
	}
}

func TestControllerFindMachineOwner(t *testing.T) {
	testMachineWithNoOwner := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineWithNoOwner",
			Namespace: "test-namespace",
		},
	}

	testMachineWithOwner := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineWithOwner",
			Namespace: "test-namespace",
			OwnerReferences: []v1.OwnerReference{{
				Kind: "MachineSet",
				UID:  uuid1,
				Name: "testMachineSet",
			}},
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		machineObjects: []runtime.Object{
			testMachineWithOwner,
			testMachineWithNoOwner,
		},
	})
	defer stop()

	// Verify machine has no owner.
	foundMachineSet, err := controller.findMachineOwner(testMachineWithNoOwner)
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if foundMachineSet != nil {
		t.Fatalf("expected no owner, got %v", foundMachineSet)
	}

	// Verify machine still has no owner as we don't have a
	// corresponding foundMachineSet in the store, even though the
	// OwnerReference is valid.
	foundMachineSet, err = controller.findMachineOwner(testMachineWithOwner)
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if foundMachineSet != nil {
		t.Fatalf("expected no owner, got %v", foundMachineSet)
	}

	testMachineSet := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineSet",
			Namespace: "test-namespace",
			UID:       uuid1,
		},
	}

	controller.machineSetInformer.Informer().GetStore().Add(testMachineSet)

	// Verify machine now has an owner
	foundMachineSet, err = controller.findMachineOwner(testMachineWithOwner)
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if foundMachineSet == nil {
		t.Fatal("expected an owner")
	}

	// Verify that a successful result returns a DeepCopy().
	if foundMachineSet == testMachineSet {
		t.Fatalf("expected a copy")
	}
}

func TestControllerFindMachineByNodeProviderID(t *testing.T) {
	testNode := &apiv1.Node{
		TypeMeta: v1.TypeMeta{
			Kind: "Node",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "ip-10-0-18-236.us-east-2.compute.internal",
		},
	}

	testMachine := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "worker-us-east-2c-p4zwl",
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		nodeObjects: []runtime.Object{
			testNode,
		},
		machineObjects: []runtime.Object{
			testMachine,
		},
	})
	defer stop()

	// Verify machine cannot be found as testNode has no
	// ProviderID and will not be indexed by the controller.
	foundMachine, err := controller.findMachineByNodeProviderID(testNode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if foundMachine != nil {
		t.Fatalf("expected nil, got %v", foundMachine)
	}

	// Update node with machine linkage.
	testNode.Spec.ProviderID = "aws:///us-east-2b/i-03759ec2e4e053f99"
	testNode.Annotations = map[string]string{
		"machine.openshift.io/machine": path.Join(testMachine.Namespace, testMachine.Name),
	}

	controller.nodeInformer.GetStore().Update(testNode)

	// Verify the machine can now be found from the information in the node
	foundMachine, err = controller.findMachineByNodeProviderID(testNode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if foundMachine == nil {
		t.Fatal("expected to find machine")
	}

	if !reflect.DeepEqual(*foundMachine, *testMachine) {
		t.Fatalf("expected %+v, got %+v", *testMachine, *foundMachine)
	}

	if foundMachine == testMachine {
		t.Fatalf("expected a copy")
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

func TestControllerNodeGroupForNodeSuccessFromMachineSet(t *testing.T) {
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

	machine := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machine",
			Namespace: "test-namespace",
			OwnerReferences: []v1.OwnerReference{{
				Name: machineSet.Name,
				Kind: machineSet.Kind,
				UID:  machineSet.UID,
			}},
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
		t.Fatal("expected no error")
	}

	if ng == nil {
		t.Fatal("expected a nodegroup")
	}

	expected := path.Join(machineSet.Namespace, machineSet.Name)
	if actual := ng.Id(); actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func TestControllerNodeGroupForNodeSuccessFromMachineDeployment(t *testing.T) {
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

	machineDeployment := &v1beta1.MachineDeployment{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machinedeployment",
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
			OwnerReferences: []v1.OwnerReference{{
				Kind: "MachineDeployment",
				Name: machineDeployment.Name,
			}},
		},
	}

	machine := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machine",
			Namespace: "test-namespace",
			OwnerReferences: []v1.OwnerReference{{
				Name: machineSet.Name,
				Kind: machineSet.Kind,
				UID:  machineSet.UID,
			}},
		},
	}

	controller, stop := mustCreateTestController(t, testControllerConfig{
		nodeObjects: []runtime.Object{
			node,
		},
		machineObjects: []runtime.Object{
			machine,
			machineSet,
			machineDeployment,
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
		t.Fatal("expected no error")
	}

	if ng == nil {
		t.Fatal("expected a nodegroup")
	}

	expected := path.Join(machineDeployment.Namespace, machineDeployment.Name)
	if actual := ng.Id(); actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func TestControllerNodeGroupsWithMachineDeployments(t *testing.T) {
	machineDeploymentTemplate := &v1beta1.MachineDeployment{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machinedeployment",
			Namespace: "test-namespace",
		},
	}

	for i, tc := range []struct {
		description string
		annotations map[string]string
		errors      bool
		nodegroups  int
	}{{
		description: "errors with bad annotations",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "-1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		nodegroups: 0,
		errors:     true,
	}, {
		description: "success but nodegroup count is 0 as deployment has no bounds",
		nodegroups:  0,
		errors:      false,
	}, {
		description: "success with valid bounds",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		nodegroups: 1,
		errors:     false,
	}} {
		t.Logf("test #%d: %s", i, tc.description)

		machineDeployment := machineDeploymentTemplate.DeepCopy()
		machineDeployment.Annotations = tc.annotations

		controller, stop := mustCreateTestController(t, testControllerConfig{
			machineObjects: []runtime.Object{
				machineDeployment,
			},
		})
		defer stop()

		nodegroups, err := controller.machineDeploymentNodeGroups()
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
}

func TestControllerNodeGroupsWithMachineSets(t *testing.T) {
	machineSetOwnedByMachineDeployment := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset-owned-by-deployment",
			Namespace: "test-namespace",
			OwnerReferences: []v1.OwnerReference{{
				Kind: "MachineDeployment",
				Name: "machinedeployment",
			}},
		},
	}

	machineDeployment := &v1beta1.MachineDeployment{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machinedeployment",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				nodeGroupMinSizeAnnotationKey: "1",
				nodeGroupMaxSizeAnnotationKey: "10",
			},
		},
	}

	machineSetTemplate := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "machineset",
			Namespace: "test-namespace",
		},
	}

	for i, tc := range []struct {
		description string
		annotations map[string]string
		errors      bool
		nodegroups  int
	}{{
		description: "errors with bad annotations",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "-1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		nodegroups: 0,
		errors:     true,
	}, {
		description: "success but nodegroup count is 0 as machineset no bounds",
		nodegroups:  0,
		errors:      false,
	}, {
		description: "success with valid machineset bounds",
		annotations: map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		},
		nodegroups: 1,
		errors:     false,
	}} {
		t.Logf("test #%d: %s", i, tc.description)

		machineSet := machineSetTemplate.DeepCopy()
		machineSet.Annotations = tc.annotations

		controller, stop := mustCreateTestController(t, testControllerConfig{
			machineObjects: []runtime.Object{
				machineDeployment,
				machineSetOwnedByMachineDeployment.DeepCopy(),
				machineSet,
			},
		})
		defer stop()

		nodegroups, err := controller.machineSetNodeGroups()
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
}

func TestControllerNodeGroupForNodeLookup(t *testing.T) {
	for i, tc := range []struct {
		description    string
		annotations    map[string]string
		lookupSucceeds bool
	}{{
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
	}} {
		test := func(t *testing.T, i int, annotations map[string]string, useMachineDeployment bool) {
			t.Helper()

			machineSet, machineDeployment := makeMachineOwner(i, 1, annotations, useMachineDeployment)

			node, machine := makeLinkedNodeAndMachine(i, v1.OwnerReference{
				Name: machineSet.Name,
				Kind: machineSet.Kind,
				UID:  machineSet.UID,
			})

			machineObjects := []runtime.Object{
				machine,
				machineSet,
			}

			if machineDeployment != nil {
				machineObjects = append(machineObjects, machineDeployment)
			}

			controller, stop := mustCreateTestController(t, testControllerConfig{
				nodeObjects:    []runtime.Object{node},
				machineObjects: machineObjects,
			})
			defer stop()

			ng, err := controller.nodeGroupForNode(node)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ng == nil && tc.lookupSucceeds {
				t.Fatalf("expected nil from lookup")
			}

			if ng != nil && !tc.lookupSucceeds {
				t.Fatalf("expected non-nil from lookup")
			}

			if tc.lookupSucceeds {
				var expected string

				if useMachineDeployment {
					expected = path.Join(machineDeployment.Namespace, machineDeployment.Name)
				} else {
					expected = path.Join(machineSet.Namespace, machineSet.Name)
				}

				if actual := ng.Id(); actual != expected {
					t.Errorf("expected %q, got %q", expected, actual)
				}
			}
		}

		t.Run(tc.description, func(t *testing.T) {
			test(t, i, tc.annotations, true) // with MachineDeployment
			test(t, i, tc.annotations, false)
		})
	}
}
