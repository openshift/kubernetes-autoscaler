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

package clusterapi

import (
	"path"
	"reflect"
	"testing"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	fakeclusterapi "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	informers "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions"
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

	testMachine := &v1alpha1.Machine{
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
