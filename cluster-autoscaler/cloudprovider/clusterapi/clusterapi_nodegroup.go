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

package clusterapi

import (
	"fmt"
	"math/rand"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	gpuapis "k8s.io/autoscaler/cluster-autoscaler/utils/gpu"
	kubeletapis "k8s.io/kubernetes/pkg/kubelet/apis"
	schedulerframework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

const (
	machineDeleteAnnotationKey = "machine.openshift.io/cluster-api-delete-machine"
	machineAnnotationKey       = "machine.openshift.io/machine"
	debugFormat                = "%s (min: %d, max: %d, replicas: %d)"

	// This default for the maximum number of pods comes from the machine-config-operator
	// see https://github.com/openshift/machine-config-operator/blob/2f1bd6d99131fa4471ed95543a51dec3d5922b2b/templates/worker/01-worker-kubelet/_base/files/kubelet.yaml#L19
	defaultMaxPods = 250
)

type nodegroup struct {
	machineController *machineController
	scalableResource  scalableResource
}

var _ cloudprovider.NodeGroup = (*nodegroup)(nil)

func (ng *nodegroup) Name() string {
	return ng.scalableResource.Name()
}

func (ng *nodegroup) Namespace() string {
	return ng.scalableResource.Namespace()
}

func (ng *nodegroup) MinSize() int {
	return ng.scalableResource.MinSize()
}

func (ng *nodegroup) MaxSize() int {
	return ng.scalableResource.MaxSize()
}

// TargetSize returns the current target size of the node group. It is
// possible that the number of nodes in Kubernetes is different at the
// moment but should be equal to Size() once everything stabilizes
// (new nodes finish startup and registration or removed nodes are
// deleted completely). Implementation required.
func (ng *nodegroup) TargetSize() (int, error) {
	size, err := ng.scalableResource.Replicas()
	if err != nil {
		return 0, err
	}
	return int(size), nil
}

// IncreaseSize increases the size of the node group. To delete a node
// you need to explicitly name it and use DeleteNode. This function
// should wait until node group size is updated. Implementation
// required.
func (ng *nodegroup) IncreaseSize(delta int) error {
	if delta <= 0 {
		return fmt.Errorf("size increase must be positive")
	}

	size, err := ng.scalableResource.Replicas()
	if err != nil {
		return err
	}
	intSize := int(size)

	if intSize+delta > ng.MaxSize() {
		return fmt.Errorf("size increase too large - desired:%d max:%d", intSize+delta, ng.MaxSize())
	}
	return ng.scalableResource.SetSize(int32(intSize + delta))
}

// DeleteNodes deletes nodes from this node group. Error is returned
// either on failure or if the given node doesn't belong to this node
// group. This function should wait until node group size is updated.
// Implementation required.
func (ng *nodegroup) DeleteNodes(nodes []*corev1.Node) error {
	ng.machineController.accessLock.Lock()
	defer ng.machineController.accessLock.Unlock()

	replicas, err := ng.scalableResource.Replicas()
	if err != nil {
		return err
	}

	// if we are at minSize already we wail early.
	if int(replicas) <= ng.MinSize() {
		return fmt.Errorf("min size reached, nodes will not be deleted")
	}

	// Step 1: Verify all nodes belong to this node group.
	for _, node := range nodes {
		actualNodeGroup, err := ng.machineController.nodeGroupForNode(node)
		if err != nil {
			return nil
		}

		if actualNodeGroup == nil {
			return fmt.Errorf("no node group found for node %q", node.Spec.ProviderID)
		}

		if actualNodeGroup.Id() != ng.Id() {
			return fmt.Errorf("node %q doesn't belong to node group %q", node.Spec.ProviderID, ng.Id())
		}
	}

	// Step 2: if deleting len(nodes) would make the replica count
	// < minSize, then the request to delete that many nodes is bogus
	// and we fail fast.
	if replicas-int32(len(nodes)) < int32(ng.MinSize()) {
		return fmt.Errorf("unable to delete %d machines in %q, machine replicas are %q, minSize is %q ", len(nodes), ng.Id(), replicas, ng.MinSize())
	}

	// Step 3: annotate the corresponding machine that it is a
	// suitable candidate for deletion and drop the replica count
	// by 1. Fail fast on any error.
	for _, node := range nodes {
		machine, err := ng.machineController.findMachineByProviderID(normalizedProviderString(node.Spec.ProviderID))
		if err != nil {
			return err
		}
		if machine == nil {
			return fmt.Errorf("unknown machine for node %q", node.Spec.ProviderID)
		}

		machine = machine.DeepCopy()

		if !machine.GetDeletionTimestamp().IsZero() {
			// The machine for this node is already being deleted
			continue
		}

		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}
		nodeGroup, err := ng.machineController.nodeGroupForNode(node)
		if err != nil {
			return err
		}

		if err := nodeGroup.scalableResource.MarkMachineForDeletion(machine); err != nil {
			return err
		}

		if err := ng.scalableResource.SetSize(replicas - 1); err != nil {
			nodeGroup.scalableResource.UnmarkMachineForDeletion(machine)
			return err
		}

		replicas--
	}

	return nil
}

// DecreaseTargetSize decreases the target size of the node group.
// This function doesn't permit to delete any existing node and can be
// used only to reduce the request for new nodes that have not been
// yet fulfilled. Delta should be negative. It is assumed that cloud
// nodegroup will not delete the existing nodes when there is an option
// to just decrease the target. Implementation required.
func (ng *nodegroup) DecreaseTargetSize(delta int) error {
	if delta >= 0 {
		return fmt.Errorf("size decrease must be negative")
	}

	size, err := ng.TargetSize()
	if err != nil {
		return err
	}

	nodes, err := ng.Nodes()
	if err != nil {
		return err
	}

	if size+delta < len(nodes) {
		return fmt.Errorf("attempt to delete existing nodes targetSize:%d delta:%d existingNodes: %d",
			size, delta, len(nodes))
	}

	return ng.scalableResource.SetSize(int32(size + delta))
}

// Id returns an unique identifier of the node group.
func (ng *nodegroup) Id() string {
	return ng.scalableResource.ID()
}

// Debug returns a string containing all information regarding this node group.
func (ng *nodegroup) Debug() string {
	replicas, err := ng.scalableResource.Replicas()
	if err != nil {
		return fmt.Sprintf("%s (min: %d, max: %d, replicas: %v)", ng.Id(), ng.MinSize(), ng.MaxSize(), err)
	}
	return fmt.Sprintf(debugFormat, ng.Id(), ng.MinSize(), ng.MaxSize(), replicas)
}

// Nodes returns a list of all nodes that belong to this node group.
// This includes instances that might have not become a kubernetes node yet.
func (ng *nodegroup) Nodes() ([]cloudprovider.Instance, error) {
	nodes, err := ng.scalableResource.Nodes()
	if err != nil {
		return nil, err
	}

	// Nodes do not have normalized IDs, so do not normalize the ID here.
	// The IDs returned here are used to check if a node is registered or not and
	// must match the ID on the Node object itself.
	// https://github.com/kubernetes/autoscaler/blob/a973259f1852303ba38a3a61eeee8489cf4e1b13/cluster-autoscaler/clusterstate/clusterstate.go#L967-L985
	instances := make([]cloudprovider.Instance, len(nodes))
	for i := range nodes {
		instances[i] = cloudprovider.Instance{
			Id: nodes[i],
		}
	}

	return instances, nil
}

// TemplateNodeInfo returns a schedulercache.NodeInfo structure of an
// empty (as if just started) node. This will be used in scale-up
// simulations to predict what would a new node look like if a node
// group was expanded. The returned NodeInfo is expected to have a
// fully populated Node object, with all of the labels, capacity and
// allocatable information as well as all pods that are started on the
// node by default, using manifest (most likely only kube-proxy).
// Implementation optional.
func (ng *nodegroup) TemplateNodeInfo() (*schedulerframework.NodeInfo, error) {
	if !ng.scalableResource.CanScaleFromZero() {
		return nil, cloudprovider.ErrNotImplemented
	}

	cpu, err := ng.scalableResource.InstanceCPUCapacity()
	if err != nil {
		return nil, err
	}

	mem, err := ng.scalableResource.InstanceMemoryCapacity()
	if err != nil {
		return nil, err
	}

	gpu, err := ng.scalableResource.InstanceGPUCapacity()
	if err != nil {
		return nil, err
	}

	pod, err := ng.scalableResource.InstanceMaxPodsCapacity()
	if err != nil {
		return nil, err
	}

	if cpu.IsZero() || mem.IsZero() {
		return nil, cloudprovider.ErrNotImplemented
	}

	if gpu.IsZero() {
		gpu = zeroQuantity.DeepCopy()
	}

	if pod.IsZero() {
		pod = *resource.NewQuantity(defaultMaxPods, resource.DecimalSI)
	}

	capacity := map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU:        cpu,
		corev1.ResourceMemory:     mem,
		corev1.ResourcePods:       pod,
		gpuapis.ResourceNvidiaGPU: gpu,
	}

	nodeName := fmt.Sprintf("%s-asg-%d", ng.Name(), rand.Int63())
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{},
		},
	}

	node.Status.Capacity = capacity
	node.Status.Allocatable = capacity
	node.Status.Conditions = cloudprovider.BuildReadyConditions()
	node.Spec.Taints = ng.scalableResource.Taints()

	node.Labels, err = ng.buildTemplateLabels(nodeName)
	if err != nil {
		return nil, err
	}

	nodeInfo := schedulerframework.NewNodeInfo(cloudprovider.BuildKubeProxy(ng.Name()))
	nodeInfo.SetNode(&node)

	return nodeInfo, nil
}

func (ng *nodegroup) buildTemplateLabels(nodeName string) (map[string]string, error) {
	labels := cloudprovider.JoinStringMaps(ng.scalableResource.Labels(), buildGenericLabels(nodeName))

	nodes, err := ng.Nodes()
	if err != nil {
		return nil, err
	}

	if len(nodes) > 0 {
		node, err := ng.machineController.findNodeByProviderID(normalizedProviderString(nodes[0].Id))
		if err != nil {
			return nil, err
		}

		if node != nil {
			labels = cloudprovider.JoinStringMaps(labels, extractNodeLabels(node))
		}
	}
	return labels, nil
}

// Exist checks if the node group really exists on the cloud nodegroup
// side. Allows to tell the theoretical node group from the real one.
// Implementation required.
func (ng *nodegroup) Exist() bool {
	return true
}

// Create creates the node group on the cloud nodegroup side.
// Implementation optional.
func (ng *nodegroup) Create() (cloudprovider.NodeGroup, error) {
	return nil, cloudprovider.ErrAlreadyExist
}

// Delete deletes the node group on the cloud nodegroup side. This will
// be executed only for autoprovisioned node groups, once their size
// drops to 0. Implementation optional.
func (ng *nodegroup) Delete() error {
	return cloudprovider.ErrNotImplemented
}

// Autoprovisioned returns true if the node group is autoprovisioned.
// An autoprovisioned group was created by CA and can be deleted when
// scaled to 0.
func (ng *nodegroup) Autoprovisioned() bool {
	return false
}

func newNodegroupFromMachineSet(controller *machineController, machineSet *MachineSet) (*nodegroup, error) {
	scalableResource, err := newMachineSetScalableResource(controller, machineSet)
	if err != nil {
		return nil, err
	}
	return &nodegroup{
		machineController: controller,
		scalableResource:  scalableResource,
	}, nil
}

func newNodegroupFromMachineDeployment(controller *machineController, machineDeployment *MachineDeployment) (*nodegroup, error) {
	scalableResource, err := newMachineDeploymentScalableResource(controller, machineDeployment)
	if err != nil {
		return nil, err
	}
	return &nodegroup{
		machineController: controller,
		scalableResource:  scalableResource,
	}, nil
}

func buildGenericLabels(nodeName string) map[string]string {
	// TODO revisit this function and add an explanation about what these
	// labels are used for, or remove them if not necessary
	m := make(map[string]string)
	m[kubeletapis.LabelArch] = cloudprovider.DefaultArch
	m[corev1.LabelArchStable] = cloudprovider.DefaultArch

	m[kubeletapis.LabelOS] = cloudprovider.DefaultOS
	m[corev1.LabelOSStable] = cloudprovider.DefaultOS

	m[corev1.LabelHostname] = nodeName
	return m
}

// extract a predefined list of labels from the existing node
func extractNodeLabels(node *corev1.Node) map[string]string {
	m := make(map[string]string)
	if node.Labels == nil {
		return m
	}

	setLabelIfNotEmpty(m, node.Labels, kubeletapis.LabelArch)
	setLabelIfNotEmpty(m, node.Labels, corev1.LabelArchStable)

	setLabelIfNotEmpty(m, node.Labels, kubeletapis.LabelOS)
	setLabelIfNotEmpty(m, node.Labels, corev1.LabelOSStable)

	setLabelIfNotEmpty(m, node.Labels, corev1.LabelInstanceType)
	setLabelIfNotEmpty(m, node.Labels, corev1.LabelInstanceTypeStable)

	setLabelIfNotEmpty(m, node.Labels, corev1.LabelZoneRegion)
	setLabelIfNotEmpty(m, node.Labels, corev1.LabelZoneRegionStable)

	setLabelIfNotEmpty(m, node.Labels, corev1.LabelZoneFailureDomain)

	return m
}

func setLabelIfNotEmpty(to, from map[string]string, key string) {
	if value := from[key]; value != "" {
		to[key] = value
	}
}
