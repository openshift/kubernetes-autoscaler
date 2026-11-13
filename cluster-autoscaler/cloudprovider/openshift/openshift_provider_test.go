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
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-autoscaler/pkg/cloudprovider"
	"sigs.k8s.io/cluster-autoscaler/pkg/config"
	coreoptions "sigs.k8s.io/cluster-autoscaler/pkg/core/options"
	"sigs.k8s.io/cluster-autoscaler/pkg/processors"
	"sigs.k8s.io/cluster-autoscaler/pkg/processors/nodegroupset"
	"sigs.k8s.io/cluster-autoscaler/pkg/simulator/framework"
	testutils "sigs.k8s.io/cluster-autoscaler/pkg/utils/test"
)

func TestConfigureNodeGroupSetProcessor(t *testing.T) {
	const machineSetLabel = "machine.openshift.io/cluster-api-machineset"
	a := testutils.BuildTestNode("worker-a", 4000, 10<<30)
	a.Labels[machineSetLabel] = "workers-a"
	b := a.DeepCopy()
	b.Name = "worker-b"
	b.Labels[machineSetLabel] = "workers-b"
	// Resource differences ensure each configured tolerance is exercised.
	b.Status.Capacity[corev1.ResourceMemory] = resource.MustParse("11Gi")
	b.Status.Allocatable[corev1.ResourceCPU] = resource.MustParse("3")
	pod := framework.NewPodInfo(testutils.BuildTestPod("workload", 1000, 0), nil)
	aInfo, bInfo := framework.NewNodeInfo(a, nil), framework.NewNodeInfo(b, nil, pod)

	for _, tc := range []struct {
		name                      string
		ignoreLabel               bool
		memory, allocatable, free float64
		wantSimilar               bool
	}{
		{"configured options", true, 0.5, 0.5, 0.5, true},
		{"ignored label missing", false, 0.5, 0.5, 0.5, false},
		{"strict memory tolerance", true, 0.01, 0.5, 0.5, false},
		{"strict allocatable tolerance", true, 0.5, 0.01, 0.5, false},
		{"strict free tolerance", true, 0.5, 0.5, 0.01, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := config.AutoscalingOptions{
				NodeGroupSetRatios: config.NodeGroupDifferenceRatios{
					MaxCapacityMemoryDifferenceRatio: tc.memory,
					MaxAllocatableDifferenceRatio:    tc.allocatable,
					MaxFreeDifferenceRatio:           tc.free,
				},
			}
			if tc.ignoreLabel {
				options.BalancingExtraIgnoredLabels = []string{machineSetLabel}
			}
			opts := &coreoptions.AutoscalerOptions{
				AutoscalingOptions: options,
				Processors:         processors.DefaultProcessors(options),
			}
			configureNodeGroupSetProcessor(opts)
			comparator := opts.Processors.NodeGroupSetProcessor.(*nodegroupset.BalancingNodeGroupSetProcessor).Comparator
			if got := comparator(aInfo, bInfo); got != tc.wantSimilar {
				t.Errorf("node similarity = %v, want %v", got, tc.wantSimilar)
			}
		})
	}
}

func TestProviderConstructorProperties(t *testing.T) {
	resourceLimits := cloudprovider.ResourceLimiter{}

	controller := NewTestMachineController(t)
	defer controller.Stop()

	provider := newProvider(&resourceLimits, controller.machineController, nil)
	if actual := provider.Name(); actual != ProviderName {
		t.Errorf("expected %q, got %q", ProviderName, actual)
	}

	rl, err := provider.GetResourceLimiter(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reflect.DeepEqual(rl, resourceLimits) {
		t.Errorf("expected %+v, got %+v", resourceLimits, rl)
	}

	if _, err := provider.Pricing(context.Background()); err != cloudprovider.ErrNotImplemented {
		t.Errorf("expected an error")
	}

	machineTypes, err := provider.GetAvailableMachineTypes(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(machineTypes) != 0 {
		t.Errorf("expected 0, got %v", len(machineTypes))
	}

	if _, err := provider.NewNodeGroup(context.Background(), "foo", nil, nil, nil, nil); err == nil {
		t.Error("expected an error")
	}

	if err := provider.Cleanup(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := provider.Refresh(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	nodegroups := provider.NodeGroups(context.Background())

	if len(nodegroups) != 0 {
		t.Errorf("expected 0, got %v", len(nodegroups))
	}

	ng, err := provider.NodeGroupForNode(context.Background(), &corev1.Node{
		TypeMeta: v1.TypeMeta{
			Kind: "Node",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "missing-node",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ng != nil {
		t.Fatalf("unexpected nodegroup: %v", ng.Id())
	}

	if got := provider.GPULabel(context.Background()); got != GPULabel {
		t.Fatalf("expected %q, got %q", GPULabel, got)
	}

	if got := len(provider.GetAvailableGPUTypes(context.Background())); got != 0 {
		t.Fatalf("expected 0 GPU types, got %d", got)
	}
}
func BenchmarkNodeGroups(b *testing.B) {
	resourceLimits := cloudprovider.ResourceLimiter{}
	annotations := map[string]string{
		nodeGroupMinSizeAnnotationKey: "1",
		nodeGroupMaxSizeAnnotationKey: "2",
	}

	controller := NewTestMachineController(b)
	defer controller.Stop()
	machineSetConfigs := NewTestConfigBuilder().
		ForMachineSet().
		WithClusterName("").
		WithNodeCount(1).
		WithAnnotations(annotations).
		BuildMultiple(100)
	if err := controller.AddTestConfigs(machineSetConfigs...); err != nil {
		b.Fatalf("unexpected error: %v", err)
	}

	provider := newProvider(&resourceLimits, controller.machineController, nil)
	if actual := provider.Name(); actual != ProviderName {
		b.Errorf("expected %q, got %q", ProviderName, actual)
	}

	b.ResetTimer()
	b.Run("NodeGroups", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			provider.NodeGroups(context.Background())
		}
	})
}
