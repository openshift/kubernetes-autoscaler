/*
Copyright 2020 The Kubernetes Authors.

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

package openshift

import (
	"fmt"
	"path"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestControllerFindMachine(t *testing.T) {
	type testCase struct {
		description    string
		name           string
		namespace      string
		useAnnotation  bool
		lookupSucceeds bool
	}

	var testCases = []testCase{{
		description:    "lookup fails",
		lookupSucceeds: false,
		useAnnotation:  false,
		name:           "machine-does-not-exist",
		namespace:      "namespace-does-not-exist",
	}, {
		description:    "lookup fails in valid namespace",
		lookupSucceeds: false,
		useAnnotation:  false,
		name:           "machine-does-not-exist-in-existing-namespace",
	}, {
		description:    "lookup succeeds",
		lookupSucceeds: true,
		useAnnotation:  false,
	}, {
		description:    "lookup succeeds with annotation",
		lookupSucceeds: true,
		useAnnotation:  true,
	}}

	test := func(t *testing.T, tc testCase, testConfig *TestConfig) {
		controller := NewTestMachineController(t)
		defer controller.Stop()
		controller.AddTestConfigs(testConfig)

		machine, err := controller.findMachine(path.Join(tc.namespace, tc.name))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tc.lookupSucceeds && machine == nil {
			t.Errorf("expected success, findMachine failed failed for %q", path.Join(tc.namespace, tc.name))
		}

		if tc.lookupSucceeds && machine != nil {
			if machine.GetName() != tc.name {
				t.Errorf("expected %q, got %q", tc.name, machine.GetName())
			}
			if machine.GetNamespace() != tc.namespace {
				t.Errorf("expected %q, got %q", tc.namespace, machine.GetNamespace())
			}
		}
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			testConfig := NewTestConfigBuilder().
				ForMachineSet().
				WithNodeCount(1).
				WithAnnotations(map[string]string{
					nodeGroupMinSizeAnnotationKey: "1",
					nodeGroupMaxSizeAnnotationKey: "10",
				}).
				Build()
			if tc.name == "" {
				tc.name = testConfig.machines[0].GetName()
			}
			if tc.namespace == "" {
				tc.namespace = testConfig.machines[0].GetNamespace()
			}
			if tc.useAnnotation {
				for i := range testConfig.machines {
					n := testConfig.nodes[i]
					annotations := n.GetAnnotations()
					_, ok := annotations[machineAnnotationKey]
					if !ok {
						t.Fatal("node did not contain machineAnnotationKey")
					}
					delete(annotations, machineAnnotationKey)
					n.SetAnnotations(annotations)
				}
			}
			test(t, tc, testConfig)
		})
	}
}

func TestControllerFindMachineOwner(t *testing.T) {
	testConfig := NewTestConfigBuilder().
		ForMachineSet().
		WithNodeCount(1).
		WithAnnotations(map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}).
		Build()

	controller := NewTestMachineController(t)
	defer controller.Stop()
	controller.AddTestConfigs(testConfig)

	testCases := []struct {
		description string
		setupFn     func() *unstructured.Unstructured
		expectNil   bool
		checkFn     func(*unstructured.Unstructured)
	}{
		{
			description: "Lookup succeeds",
			setupFn: func() *unstructured.Unstructured {
				return testConfig.machines[0].DeepCopy()
			},
			expectNil: false,
			checkFn: func(testResult *unstructured.Unstructured) {
				if testConfig.spec.machineSetName != testResult.GetName() {
					t.Errorf("expected %q, got %q", testConfig.spec.machineSetName, testResult.GetName())
				}
			},
		},
		{
			description: "Lookup fails as the machine ownerref Name != machineset Name",
			setupFn: func() *unstructured.Unstructured {
				testMachine := testConfig.machines[0].DeepCopy()
				ownerRefs := testMachine.GetOwnerReferences()
				ownerRefs[0].Name = "does-not-match-machineset"
				testMachine.SetOwnerReferences(ownerRefs)

				return testMachine
			},
			expectNil: true,
		},
		{
			description: "Delete the MachineSet and lookup should fail",
			setupFn: func() *unstructured.Unstructured {
				if err := controller.DeleteResource(controller.machineSetInformer, controller.machineSetResource, testConfig.machineSet); err != nil {
					t.Fatalf("unexpected error, got %v", err)
				}

				return testConfig.machines[0].DeepCopy()
			},
			expectNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			if tc.setupFn == nil {
				t.Fatalf("setupFn must be set")
			}
			testMachine := tc.setupFn()
			testResult, err := controller.findMachineOwner(testMachine)
			if err != nil {
				t.Fatalf("unexpected error, got %v", err)
			}
			if testResult == nil {
				if !tc.expectNil {
					t.Fatal("expected findMachineOwner to return non-nil")
				}
			} else {
				if tc.expectNil {
					t.Fatalf("expected nil result from findMachineOwner, got Machine %q", testResult.GetName())
				}
			}
			if tc.checkFn != nil {
				tc.checkFn(testResult)
			}
		})
	}
}

func TestControllerFindMachineByProviderID(t *testing.T) {
	testConfig := NewTestConfigBuilder().
		ForMachineSet().
		WithNodeCount(1).
		WithAnnotations(map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}).
		Build()

	controller := NewTestMachineController(t)
	defer controller.Stop()
	controller.AddTestConfigs(testConfig)

	// Remove all the "machine" annotation values on all the
	// nodes. We want to force findMachineByProviderID() to only
	// be successful by searching on provider ID.
	for _, node := range testConfig.nodes {
		delete(node.Annotations, machineAnnotationKey)
		if err := controller.nodeInformer.GetStore().Update(node); err != nil {
			t.Fatalf("unexpected error updating node, got %v", err)
		}
	}

	// Test #1: Verify underlying machine provider ID matches
	machine, err := controller.findMachineByProviderID(normalizedProviderString(testConfig.nodes[0].Spec.ProviderID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if machine == nil {
		t.Fatal("expected to find machine")
	}

	if !reflect.DeepEqual(machine, testConfig.machines[0]) {
		t.Fatalf("expected machines to be equal - expected %+v, got %+v", testConfig.machines[0], machine)
	}

	// Test #2: Verify machine returned by fake provider ID is correct machine
	fakeProviderID := fmt.Sprintf("%s/%s", testConfig.machines[0].GetNamespace(), testConfig.machines[0].GetName())
	machine, err = controller.findMachineByProviderID(normalizedProviderID(fakeProviderID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if machine != nil {
		t.Fatal("expected find to fail")
	}

	// Test #3: Verify machine is not found if it has a
	// non-existent or different provider ID.
	machine = testConfig.machines[0].DeepCopy()
	if err := unstructured.SetNestedField(machine.Object, "does-not-match", "spec", "providerID"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := controller.UpdateResource(controller.machineInformer, controller.machineResource, machine); err != nil {
		t.Fatalf("unexpected error updating machine, got %v", err)
	}

	machine, err = controller.findMachineByProviderID(normalizedProviderString(testConfig.nodes[0].Spec.ProviderID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if machine != nil {
		t.Fatal("expected find to fail")
	}
}

func TestControllerFindNodeByNodeName(t *testing.T) {
	testConfig := NewTestConfigBuilder().
		ForMachineSet().
		WithNodeCount(1).
		WithAnnotations(map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}).
		Build()

	controller := NewTestMachineController(t)
	defer controller.Stop()
	controller.AddTestConfigs(testConfig)

	testCases := []struct {
		description string
		nodeName    string
		expectNil   bool
	}{
		{
			description: "Verify known node can be found",
			nodeName:    testConfig.nodes[0].Name,
			expectNil:   false,
		},
		{
			description: "Verify non-existent node cannot be found",
			nodeName:    testConfig.nodes[0].Name + "non-existent",
			expectNil:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			node, err := controller.findNodeByNodeName(tc.nodeName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if node == nil {
				if !tc.expectNil {
					t.Fatal("expected findNodeByNodeName to return non-nil")
				}
			} else {
				if tc.expectNil {
					t.Fatalf("expected findNodeByNodeName to return nil, got %q", node.GetName())
				}
			}
		})
	}
}

func TestControllerListMachinesForScalableResource(t *testing.T) {
	test := func(t *testing.T, testConfig1 *TestConfig, testConfig2 *TestConfig) {
		controller := NewTestMachineController(t)
		defer controller.Stop()
		controller.AddTestConfigs(testConfig1)

		if err := controller.AddTestConfigs(testConfig2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		scalableResource1 := testConfig1.machineSet

		scalableResource2 := testConfig2.machineSet

		machinesInScalableResource1, err := controller.listMachinesForScalableResource(scalableResource1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		machinesInScalableResource2, err := controller.listMachinesForScalableResource(scalableResource2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !assert.ElementsMatch(t, testConfig1.machines, machinesInScalableResource1) {
			t.Fatalf("expected %+v, got %+v", testConfig1.machines, machinesInScalableResource1)
		}
		if !assert.ElementsMatch(t, testConfig2.machines, machinesInScalableResource2) {
			t.Fatalf("expected %+v, got %+v", testConfig1.machines, machinesInScalableResource2)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		clusterName := RandomString(6)
		testConfig1 := NewTestConfigBuilder().
			ForMachineSet().
			WithClusterName(clusterName).
			WithNodeCount(5).
			WithAnnotations(map[string]string{
				nodeGroupMinSizeAnnotationKey: "1",
				nodeGroupMaxSizeAnnotationKey: "10",
			}).
			Build()

		// Construct a second set of objects and add the machines,
		// nodes and the additional machineset to the existing set of
		// test objects in the controller. This gives us two
		// machinesets, each with their own machines and linked nodes.
		testConfig2 := NewTestConfigBuilder().
			ForMachineSet().
			WithClusterName(clusterName).
			WithNodeCount(5).
			WithAnnotations(map[string]string{
				nodeGroupMinSizeAnnotationKey: "1",
				nodeGroupMaxSizeAnnotationKey: "10",
			}).
			Build()

		test(t, testConfig1, testConfig2)
	})
}

func TestControllerLookupNodeGroupForNonExistentNode(t *testing.T) {
	test := func(t *testing.T, testConfig *TestConfig) {
		controller := NewTestMachineController(t)
		defer controller.Stop()
		controller.AddTestConfigs(testConfig)

		node := testConfig.nodes[0].DeepCopy()
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

	t.Run("MachineSet", func(t *testing.T) {
		testConfig := NewTestConfigBuilder().
			ForMachineSet().
			WithNodeCount(1).
			WithAnnotations(map[string]string{
				nodeGroupMinSizeAnnotationKey: "1",
				nodeGroupMaxSizeAnnotationKey: "10",
			}).
			Build()
		test(t, testConfig)
	})
}

func TestControllerNodeGroupForNodeWithMissingMachineOwner(t *testing.T) {
	test := func(t *testing.T, testConfig *TestConfig) {
		controller := NewTestMachineController(t)
		defer controller.Stop()
		controller.AddTestConfigs(testConfig)

		machine := testConfig.machines[0].DeepCopy()
		machine.SetOwnerReferences([]metav1.OwnerReference{})

		if err := controller.UpdateResource(controller.machineInformer, controller.machineResource, machine); err != nil {
			t.Fatalf("unexpected error updating machine, got %v", err)
		}

		ng, err := controller.nodeGroupForNode(testConfig.nodes[0])
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ng != nil {
			t.Fatalf("unexpected nodegroup: %v", ng)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		testConfig := NewTestConfigBuilder().
			ForMachineSet().
			WithNodeCount(1).
			WithAnnotations(map[string]string{
				nodeGroupMinSizeAnnotationKey: "1",
				nodeGroupMaxSizeAnnotationKey: "10",
			}).
			Build()
		test(t, testConfig)
	})
}

func TestControllerNodeGroupForNodeWithPositiveScalingBounds(t *testing.T) {
	test := func(t *testing.T, testConfig *TestConfig) {
		controller := NewTestMachineController(t)
		defer controller.Stop()
		controller.AddTestConfigs(testConfig)

		ng, err := controller.nodeGroupForNode(testConfig.nodes[0])
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// We allow scaling if minSize=maxSize
		if ng == nil {
			t.Fatalf("unexpected nodegroup: %v", ng)
		}
	}

	t.Run("MachineSet", func(t *testing.T) {
		testConfig := NewTestConfigBuilder().
			ForMachineSet().
			WithNodeCount(1).
			WithAnnotations(map[string]string{
				nodeGroupMinSizeAnnotationKey: "1",
				nodeGroupMaxSizeAnnotationKey: "1",
			}).
			Build()
		test(t, testConfig)
	})
}

func TestControllerNodeGroups(t *testing.T) {
	assertNodegroupLen := func(t *testing.T, controller *testMachineController, expected int) {
		t.Helper()
		nodegroups, err := controller.nodeGroups()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := len(nodegroups); got != expected {
			t.Fatalf("expected %d, got %d", expected, got)
		}
	}

	annotations := map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "2",
	}

	controller := NewTestMachineController(t)
	defer controller.Stop()

	clusterName := RandomString(6)

	// Test #1: zero nodegroups
	assertNodegroupLen(t, controller, 0)

	// Test #2: add 5 machineset-based nodegroups
	machineSetConfigs := NewTestConfigBuilder().
		ForMachineSet().
		WithClusterName(clusterName).
		WithNodeCount(1).
		WithAnnotations(annotations).
		BuildMultiple(5)
	if err := controller.AddTestConfigs(machineSetConfigs...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNodegroupLen(t, controller, 5)

	// Test #3: delete 5 machineset-backed objects
	if err := controller.DeleteTestConfigs(machineSetConfigs...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNodegroupLen(t, controller, 0)

	// Test #4: 5 machinesets with minSize=maxSize results in 5 nodegroups
	annotations = map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "1",
	}

	machineSetConfigs = NewTestConfigBuilder().
		ForMachineSet().
		WithClusterName(clusterName).
		WithNodeCount(1).
		WithAnnotations(annotations).
		BuildMultiple(5)
	if err := controller.AddTestConfigs(machineSetConfigs...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNodegroupLen(t, controller, 5)
	// clean up
	if err := controller.DeleteTestConfigs(machineSetConfigs...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test #5: machineset with bad scaling bounds results in an error and no nodegroups
	annotations = map[string]string{
		nodeGroupMinSizeAnnotationKey: "-1",
		nodeGroupMaxSizeAnnotationKey: "1",
	}

	machineSetConfigs = NewTestConfigBuilder().
		ForMachineSet().
		WithClusterName(clusterName).
		WithNodeCount(5).
		WithAnnotations(annotations).
		BuildMultiple(1)
	if err := controller.AddTestConfigs(machineSetConfigs...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := controller.nodeGroups(); err == nil {
		t.Fatalf("expected an error")
	}
	// clean up
	if err := controller.DeleteTestConfigs(machineSetConfigs...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test #6: machineset that is non-authoritative results in no nodegroups
	annotations = map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "2",
	}

	machineSetConfigs = NewTestConfigBuilder().
		ForMachineSet().
		WithClusterName(clusterName).
		WithNodeCount(1).
		WithAnnotations(annotations).
		WithAuthoritativeAPI("ClusterAPI").
		BuildMultiple(1)
	if err := controller.AddTestConfigs(machineSetConfigs...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNodegroupLen(t, controller, 0)
}

func TestControllerNodeGroupsNodeCount(t *testing.T) {
	type testCase struct {
		nodeGroups            int
		nodesPerGroup         int
		expectedNodeGroups    int
		expectedNodesPerGroup int
	}

	var testCases = []testCase{{
		nodeGroups:            0,
		nodesPerGroup:         0,
		expectedNodeGroups:    0,
		expectedNodesPerGroup: 0,
	}, {
		nodeGroups:            1,
		nodesPerGroup:         0,
		expectedNodeGroups:    0,
		expectedNodesPerGroup: 0,
	}, {
		nodeGroups:            2,
		nodesPerGroup:         10,
		expectedNodeGroups:    2,
		expectedNodesPerGroup: 10,
	}}

	test := func(t *testing.T, tc testCase, testConfigs []*TestConfig) {
		controller := NewTestMachineController(t)
		defer controller.Stop()
		controller.AddTestConfigs(testConfigs...)

		nodegroups, err := controller.nodeGroups()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := len(nodegroups); got != tc.expectedNodeGroups {
			t.Fatalf("expected %d, got %d", tc.expectedNodeGroups, got)
		}

		for i := range nodegroups {
			nodes, err := nodegroups[i].Nodes()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(nodes); got != tc.expectedNodesPerGroup {
				t.Fatalf("expected %d, got %d", tc.expectedNodesPerGroup, got)
			}
		}
	}

	annotations := map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "10",
	}

	t.Run("MachineSet", func(t *testing.T) {
		for _, tc := range testCases {
			testConfigs := NewTestConfigBuilder().
				ForMachineSet().
				WithNodeCount(tc.nodesPerGroup).
				WithAnnotations(annotations).
				BuildMultiple(tc.nodeGroups)
			test(t, tc, testConfigs)
		}
	})
}

func TestControllerFindMachineFromNodeAnnotation(t *testing.T) {
	testConfig := NewTestConfigBuilder().
		ForMachineSet().
		WithNodeCount(1).
		WithAnnotations(map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}).
		Build()

	controller := NewTestMachineController(t)
	defer controller.Stop()
	controller.AddTestConfigs(testConfig)

	// Remove all the provider ID values on all the machines. We
	// want to force findMachineByProviderID() to fallback to
	// searching using the annotation on the node object.
	for _, machine := range testConfig.machines {
		unstructured.RemoveNestedField(machine.Object, "spec", "providerID")

		if err := controller.UpdateResource(controller.machineInformer, controller.machineResource, machine); err != nil {
			t.Fatalf("unexpected error updating machine, got %v", err)
		}
	}

	// Test #1: Verify machine can be found from node annotation
	machine, err := controller.findMachineByProviderID(normalizedProviderString(testConfig.nodes[0].Spec.ProviderID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if machine == nil {
		t.Fatal("expected to find machine")
	}
	if !reflect.DeepEqual(machine, testConfig.machines[0]) {
		t.Fatalf("expected machines to be equal - expected %+v, got %+v", testConfig.machines[0], machine)
	}

	// Test #2: Verify machine is not found if it has no
	// corresponding machine annotation.
	node := testConfig.nodes[0].DeepCopy()
	delete(node.Annotations, machineAnnotationKey)
	if err := controller.nodeInformer.GetStore().Update(node); err != nil {
		t.Fatalf("unexpected error updating node, got %v", err)
	}
	machine, err = controller.findMachineByProviderID(normalizedProviderString(testConfig.nodes[0].Spec.ProviderID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if machine != nil {
		t.Fatal("expected find to fail")
	}
}

func TestControllerMachineSetNodeNamesWithoutLinkage(t *testing.T) {
	testConfig := NewTestConfigBuilder().
		ForMachineSet().
		WithNodeCount(3).
		WithAnnotations(map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}).
		Build()

	controller := NewTestMachineController(t)
	defer controller.Stop()
	controller.AddTestConfigs(testConfig)

	// Remove all linkage between node and machine.
	for i := range testConfig.machines {
		machine := testConfig.machines[i].DeepCopy()

		unstructured.RemoveNestedField(machine.Object, "spec", "providerID")
		unstructured.RemoveNestedField(machine.Object, "status", "nodeRef")

		if err := controller.UpdateResource(controller.machineInformer, controller.machineResource, machine); err != nil {
			t.Fatalf("unexpected error updating machine, got %v", err)
		}
	}

	nodegroups, err := controller.nodeGroups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if l := len(nodegroups); l != 1 {
		t.Fatalf("expected 1 nodegroup, got %d", l)
	}

	ng := nodegroups[0]
	nodeNames, err := ng.Nodes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeCount := 0
	for _, node := range nodeNames {
		if !isPendingMachineProviderID(normalizedProviderString(node.Id)) {
			nodeCount++
		}
	}

	// We removed all linkage - so we should get 0 nodes back.
	if nodeCount != 0 {
		t.Fatalf("expected len=0, got len=%v", nodeCount)
	}
}

func TestControllerMachineSetNodeNamesUsingProviderID(t *testing.T) {
	testConfig := NewTestConfigBuilder().
		ForMachineSet().
		WithNodeCount(3).
		WithAnnotations(map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}).
		Build()

	controller := NewTestMachineController(t)
	defer controller.Stop()
	controller.AddTestConfigs(testConfig)

	nodegroups, err := controller.nodeGroups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if l := len(nodegroups); l != 1 {
		t.Fatalf("expected 1 nodegroup, got %d", l)
	}

	ng := nodegroups[0]
	nodeNames, err := ng.Nodes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nodeNames) != len(testConfig.nodes) {
		t.Fatalf("expected len=%v, got len=%v", len(testConfig.nodes), len(nodeNames))
	}

	sort.Slice(nodeNames, func(i, j int) bool {
		return nodeNames[i].Id < nodeNames[j].Id
	})

	for i := range testConfig.nodes {
		if nodeNames[i].Id != testConfig.nodes[i].Spec.ProviderID {
			t.Fatalf("expected %q, got %q", testConfig.nodes[i].Spec.ProviderID, nodeNames[i].Id)
		}
	}
}

func TestControllerMachineSetNodeNamesUsingStatusNodeRefName(t *testing.T) {
	testConfig := NewTestConfigBuilder().
		ForMachineSet().
		WithNodeCount(3).
		WithAnnotations(map[string]string{
			nodeGroupMinSizeAnnotationKey: "1",
			nodeGroupMaxSizeAnnotationKey: "10",
		}).
		Build()

	controller := NewTestMachineController(t)
	defer controller.Stop()
	controller.AddTestConfigs(testConfig)

	// Remove all the provider ID values on all the machines. We
	// want to force machineSetNodeNames() to fallback to
	// searching using Status.NodeRef.Name.
	for i := range testConfig.machines {
		machine := testConfig.machines[i].DeepCopy()

		unstructured.RemoveNestedField(machine.Object, "spec", "providerID")

		if err := controller.UpdateResource(controller.machineInformer, controller.machineResource, machine); err != nil {
			t.Fatalf("unexpected error updating machine, got %v", err)
		}
	}

	nodegroups, err := controller.nodeGroups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if l := len(nodegroups); l != 1 {
		t.Fatalf("expected 1 nodegroup, got %d", l)
	}

	nodeNames, err := nodegroups[0].Nodes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nodeNames) != len(testConfig.nodes) {
		t.Fatalf("expected len=%v, got len=%v", len(testConfig.nodes), len(nodeNames))
	}

	sort.Slice(nodeNames, func(i, j int) bool {
		return nodeNames[i].Id < nodeNames[j].Id
	})

	for i := range testConfig.nodes {
		if nodeNames[i].Id != testConfig.nodes[i].Spec.ProviderID {
			t.Fatalf("expected %q, got %q", testConfig.nodes[i].Spec.ProviderID, nodeNames[i].Id)
		}
	}
}

func TestIsFailedMachineProviderID(t *testing.T) {
	testCases := []struct {
		name       string
		providerID normalizedProviderID
		expected   bool
	}{
		{
			name:       "with the failed machine prefix",
			providerID: normalizedProviderID(fmt.Sprintf("%sfoo", failedMachinePrefix)),
			expected:   true,
		},
		{
			name:       "without the failed machine prefix",
			providerID: normalizedProviderID("foo"),
			expected:   false,
		},
		{
			name:       "with provider ID created by createFailedMachineNormalizedProviderID should return true",
			providerID: normalizedProviderID(createFailedMachineNormalizedProviderID("cluster-api", "id-0001")),
			expected:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFailedMachineProviderID(tc.providerID); got != tc.expected {
				t.Errorf("test case: %s, expected: %v, got: %v", tc.name, tc.expected, got)
			}
		})
	}
}

func TestMachineKeyFromFailedProviderID(t *testing.T) {
	testCases := []struct {
		name       string
		providerID normalizedProviderID
		expected   string
	}{
		{
			name:       "with a valid failed machine prefix",
			providerID: normalizedProviderID(fmt.Sprintf("%stest-namespace_foo", failedMachinePrefix)),
			expected:   "test-namespace/foo",
		},
		{
			name:       "with a machine with an underscore in the name",
			providerID: normalizedProviderID(fmt.Sprintf("%stest-namespace_foo_bar", failedMachinePrefix)),
			expected:   "test-namespace/foo_bar",
		},
		{
			name:       "with a provider ID created by createFailedMachineNormalizedProviderID",
			providerID: normalizedProviderID(createFailedMachineNormalizedProviderID("cluster-api", "id-0001")),
			expected:   "cluster-api/id-0001",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := machineKeyFromFailedProviderID(tc.providerID); got != tc.expected {
				t.Errorf("test case: %s, expected: %q, got: %q", tc.name, tc.expected, got)
			}
		})
	}
}

func Test_isDeletingMachineProviderID(t *testing.T) {
	type testCase struct {
		description    string
		providerID     string
		expectedReturn bool
	}

	testCases := []testCase{
		{
			description:    "proper provider ID without deletion prefix should return false",
			providerID:     "fake-provider://a.provider.id-0001",
			expectedReturn: false,
		},
		{
			description:    "provider ID with deletion prefix should return true",
			providerID:     fmt.Sprintf("%s%s_%s", deletingMachinePrefix, "cluster-api", "id-0001"),
			expectedReturn: true,
		},
		{
			description:    "empty provider ID should return false",
			providerID:     "",
			expectedReturn: false,
		},
		{
			description:    "provider ID created with createDeletingMachineNormalizedProviderID returns true",
			providerID:     createDeletingMachineNormalizedProviderID("cluster-api", "id-0001"),
			expectedReturn: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			observed := isDeletingMachineProviderID(normalizedProviderID(tc.providerID))
			if observed != tc.expectedReturn {
				t.Fatalf("unexpected return for provider ID %q, expected %t, observed %t", tc.providerID, tc.expectedReturn, observed)
			}
		})
	}

}

// TestNodeHasValidProviderID tests all permutations of provider IDs
// to determine whether the providerID is the standard cloud provider ID
// or has been modified by CAS CAPI provider
func TestNodeHasValidProviderID(t *testing.T) {
	type testCase struct {
		description    string
		providerID     normalizedProviderID
		expectedReturn bool
	}

	testCases := []testCase{
		{
			description:    "real looking provider ID should return true",
			providerID:     normalizedProviderID("fake-provider://a.provider.id-0001"),
			expectedReturn: true,
		},
		{
			description:    "provider ID created with createDeletingMachineNormalizedProviderID should return false",
			providerID:     normalizedProviderID(createDeletingMachineNormalizedProviderID("cluster-api", "id-0001")),
			expectedReturn: false,
		},
		{
			description:    "provider ID created with createPendingDeletionMachineNormalizedProviderID should return false",
			providerID:     normalizedProviderID(createPendingMachineProviderID("cluster-api", "id-0001")),
			expectedReturn: false,
		},
		{
			description:    "provider ID created with createFailedMachineNormalizedProviderID should return false",
			providerID:     normalizedProviderID(createFailedMachineNormalizedProviderID("cluster-api", "id-0001")),
			expectedReturn: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			observed := isProviderIDNormalized(tc.providerID)
			if observed != tc.expectedReturn {
				t.Fatalf("unexpected return for provider ID %q, expected %t, observed %t", tc.providerID, tc.expectedReturn, observed)
			}
		})
	}
}

func Test_machineKeyFromDeletingMachineProviderID(t *testing.T) {
	type testCase struct {
		description    string
		providerID     string
		expectedReturn string
	}

	testCases := []testCase{
		{
			description:    "real looking provider ID with no deletion prefix returns provider id",
			providerID:     "fake-provider://a.provider.id-0001",
			expectedReturn: "fake-provider://a.provider.id-0001",
		},
		{
			description:    "namespace_name provider ID with no deletion prefix returns proper provider ID",
			providerID:     "cluster-api_id-0001",
			expectedReturn: "cluster-api/id-0001",
		},
		{
			description:    "namespace_name provider ID with deletion prefix returns proper provider ID",
			providerID:     fmt.Sprintf("%s%s_%s", deletingMachinePrefix, "cluster-api", "id-0001"),
			expectedReturn: "cluster-api/id-0001",
		},
		{
			description:    "namespace_name provider ID with deletion prefix and two underscores returns proper provider ID",
			providerID:     fmt.Sprintf("%s%s_%s", deletingMachinePrefix, "cluster-api", "id_0001"),
			expectedReturn: "cluster-api/id_0001",
		},
		{
			description:    "provider ID created with createDeletingMachineNormalizedProviderID returns proper provider ID",
			providerID:     createDeletingMachineNormalizedProviderID("cluster-api", "id-0001"),
			expectedReturn: "cluster-api/id-0001",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			observed := machineKeyFromDeletingMachineProviderID(normalizedProviderID(tc.providerID))
			if observed != tc.expectedReturn {
				t.Fatalf("unexpected return for provider ID %q, expected %q, observed %q", tc.providerID, tc.expectedReturn, observed)
			}
		})
	}
}

func Test_createDeletingMachineNormalizedProviderID(t *testing.T) {
	type testCase struct {
		description    string
		namespace      string
		name           string
		expectedReturn string
	}

	testCases := []testCase{
		{
			description:    "namespace and name return proper normalized ID",
			namespace:      machineAPINamespace,
			name:           "id-0001",
			expectedReturn: fmt.Sprintf("%s%s_%s", deletingMachinePrefix, machineAPINamespace, "id-0001"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			observed := createDeletingMachineNormalizedProviderID(tc.namespace, tc.name)
			if observed != tc.expectedReturn {
				t.Fatalf("unexpected return for (namespace %q, name %q), expected %q, observed %q", tc.namespace, tc.name, tc.expectedReturn, observed)
			}
		})
	}
}

// Test_createPendingMachineProviderID tests the creation of a pending machine provider ID
func Test_createPendingMachineProviderID(t *testing.T) {
	type testCase struct {
		description    string
		namespace      string
		name           string
		expectedReturn string
	}

	testCases := []testCase{
		{
			description:    "namespace and name return proper normalized ID",
			namespace:      machineAPINamespace,
			name:           "id-0001",
			expectedReturn: fmt.Sprintf("%s%s_%s", pendingMachinePrefix, machineAPINamespace, "id-0001"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			observed := createPendingMachineProviderID(tc.namespace, tc.name)
			if observed != tc.expectedReturn {
				t.Fatalf("unexpected return for (namespace %q, name %q), expected %q, observed %q", tc.namespace, tc.name, tc.expectedReturn, observed)
			}
		})
	}
}
