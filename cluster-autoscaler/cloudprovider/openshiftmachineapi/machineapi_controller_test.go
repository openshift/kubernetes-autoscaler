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
	informers "github.com/openshift/cluster-api/pkg/client/informers_generated/externalversions"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/controller"
)

func mustCreateTestController(t *testing.T) *machineController {
	t.Helper()

	kubeclientSet := fakekube.NewSimpleClientset()
	clusterclientSet := fakeclusterapi.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclientSet, controller.NoResyncPeriodFunc())
	clusterInformerFactory := informers.NewSharedInformerFactory(clusterclientSet, controller.NoResyncPeriodFunc())

	controller, err := newMachineController(kubeInformerFactory, clusterInformerFactory)
	if err != nil {
		t.Fatalf("failed to create test controller")
	}

	if err := controller.run(make(chan struct{})); err != nil {
		t.Fatalf("failed to run controller: %v", err)
	}

	return controller
}

func TestFindMachineByID(t *testing.T) {
	controller := mustCreateTestController(t)

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

func TestFindMachineOwner(t *testing.T) {
	controller := mustCreateTestController(t)

	testMachineWithNoOwner := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineWithNoOwner",
			Namespace: "testNamespace",
		},
	}

	testMachineWithOwner := &v1beta1.Machine{
		TypeMeta: v1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineWithOwner",
			Namespace: "testNamespace",
			OwnerReferences: []v1.OwnerReference{{
				Kind: "MachineSet",
				UID:  "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
				Name: "testMachineSet",
			}},
		},
	}

	controller.machineInformer.Informer().GetStore().Add(testMachineWithOwner)
	controller.machineInformer.Informer().GetStore().Add(testMachineWithNoOwner)

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
			Namespace: "testNamespace",
			UID:       "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
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

func TestFindMachineByNodeProviderID(t *testing.T) {
	controller := mustCreateTestController(t)

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

	controller.nodeInformer.GetStore().Add(testNode)
	controller.machineInformer.Informer().GetStore().Add(testMachine)

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

func TestFindNodeByNodeName(t *testing.T) {
	controller := mustCreateTestController(t)

	testNode := &apiv1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: "ip-10-0-18-236.us-east-2.compute.internal",
		},
	}

	controller.nodeInformer.GetStore().Add(testNode)

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

func TestMachinesInMachineSet(t *testing.T) {
	controller := mustCreateTestController(t)

	testMachineSet1 := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineSet1",
			Namespace: "testNamespace",
			UID:       "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
		},
	}

	testMachineSet2 := &v1beta1.MachineSet{
		TypeMeta: v1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testMachineSet2",
			Namespace: "testNamespace",
			UID:       "abcdef12-a3d5-a45f-887b-6b49aa8fc218",
		},
	}

	controller.machineSetInformer.Informer().GetStore().Add(testMachineSet1)
	controller.machineSetInformer.Informer().GetStore().Add(testMachineSet2)

	testMachines := make([]*v1beta1.Machine, 10)

	for i := 0; i < 10; i++ {
		testMachines[i] = &v1beta1.Machine{
			TypeMeta: v1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("machine-%d", i),
				Namespace: "testNamespace",
			},
		}
		// Only even numbered machines belong to testMachineSet1
		if i%2 == 0 {
			testMachines[i].OwnerReferences = []v1.OwnerReference{{
				Kind: "MachineSet",
				UID:  "ec21c5fb-a3d5-a45f-887b-6b49aa8fc218",
				Name: "testMachineSet1",
			}}
		}
		controller.machineInformer.Informer().GetStore().Add(testMachines[i])
	}

	foundMachines, err := controller.MachinesInMachineSet(testMachineSet1)
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
