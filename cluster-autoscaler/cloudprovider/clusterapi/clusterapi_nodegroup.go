/*
Copyright 2018 The Kubernetes Authors.

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
	"path"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	kubeclient "k8s.io/client-go/kubernetes"
	schedulercache "k8s.io/kubernetes/pkg/scheduler/cache"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

const (
	machineDeleteAnnotationKey = "sigs.k8s.io/cluster-api-delete-machine"
)

type scalableResource interface {
	ID() string
	MaxSize() int
	MinSize() int
	Name() string
	Namespace() string
	Nodes() ([]string, error)
	SetSize(nreplicas int32) error
	Replicas() int32
}

type machineSetScalableResource struct {
	clusterapiClient clusterv1alpha1.ClusterV1alpha1Interface
	controller       *machineController
	machineSet       *v1alpha1.MachineSet
	maxSize          int
	minSize          int
}

type machineDeploymentScalableResource struct {
	clusterapiClient  clusterv1alpha1.ClusterV1alpha1Interface
	controller        *machineController
	machineDeployment *v1alpha1.MachineDeployment
	maxSize           int
	minSize           int
}

type nodegroup struct {
	clusterapiClient  clusterv1alpha1.ClusterV1alpha1Interface
	kubeclient        kubeclient.Interface
	machineController *machineController
	scalableResource  scalableResource
}

var _ cloudprovider.NodeGroup = (*nodegroup)(nil)
var _ scalableResource = (*machineSetScalableResource)(nil)
var _ scalableResource = (*machineDeploymentScalableResource)(nil)

func newNodegroupFromMachineSet(controller *machineController, machineSet *v1alpha1.MachineSet) (*nodegroup, error) {
	scalableResource, err := newMachineSetScalableResource(controller, machineSet)
	if err != nil {
		return nil, err
	}
	return &nodegroup{
		clusterapiClient:  controller.clusterClientset.ClusterV1alpha1(),
		machineController: controller,
		scalableResource:  scalableResource,
	}, nil
}

func newNodegroupFromMachineDeployment(controller *machineController, machineDeployment *v1alpha1.MachineDeployment) (*nodegroup, error) {
	scalableResource, err := newMachineDeploymentScalableResource(controller, machineDeployment)
	if err != nil {
		return nil, err
	}
	return &nodegroup{
		clusterapiClient:  controller.clusterClientset.ClusterV1alpha1(),
		machineController: controller,
		scalableResource:  scalableResource,
	}, nil
}

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
	return int(ng.scalableResource.Replicas()), nil
}

// IncreaseSize increases the size of the node group. To delete a node
// you need to explicitly name it and use DeleteNode. This function
// should wait until node group size is updated. Implementation
// required.
func (ng *nodegroup) IncreaseSize(delta int) error {
	if delta <= 0 {
		return fmt.Errorf("size increase must be positive")
	}
	size := int(ng.scalableResource.Replicas())
	if size+delta > ng.MaxSize() {
		return fmt.Errorf("size increase too large - desired:%d max:%d", size+delta, ng.MaxSize())
	}
	return ng.scalableResource.SetSize(int32(size + delta))
}

// DeleteNodes deletes nodes from this node group. Error is returned
// either on failure or if the given node doesn't belong to this node
// group. This function should wait until node group size is updated.
// Implementation required.
func (ng *nodegroup) DeleteNodes(nodes []*apiv1.Node) error {
	for _, node := range nodes {
		machine, err := ng.machineController.findMachineByNodeProviderID(node)
		if err != nil {
			return err
		}
		if machine == nil {
			return fmt.Errorf("unknown machine for node %q", node.Spec.ProviderID)
		}

		machine = machine.DeepCopy()

		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}

		machine.Annotations[machineDeleteAnnotationKey] = time.Now().String()

		if _, err := ng.clusterapiClient.Machines(machine.Namespace).Update(machine); err != nil {
			return fmt.Errorf("failed to update machine %s/%s: %v", machine.Namespace, machine.Name, err)
		}
	}

	if int(ng.scalableResource.Replicas())-len(nodes) <= 0 {
		return fmt.Errorf("unable to delete %d machines in %q, machine replicas are <= 0 ", len(nodes), ng.Id())
	}

	return ng.scalableResource.SetSize(ng.scalableResource.Replicas() - int32(len(nodes)))
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
	return fmt.Sprintf("%s (min: %d, max: %d, replicas: %d)", ng.Id(), ng.MinSize(), ng.MaxSize(), ng.scalableResource.Replicas())
}

// Nodes returns a list of all nodes that belong to this node group.
func (ng *nodegroup) Nodes() ([]string, error) {
	return ng.scalableResource.Nodes()
}

// TemplateNodeInfo returns a schedulercache.NodeInfo structure of an
// empty (as if just started) node. This will be used in scale-up
// simulations to predict what would a new node look like if a node
// group was expanded. The returned NodeInfo is expected to have a
// fully populated Node object, with all of the labels, capacity and
// allocatable information as well as all pods that are started on the
// node by default, using manifest (most likely only kube-proxy).
// Implementation optional.
func (ng *nodegroup) TemplateNodeInfo() (*schedulercache.NodeInfo, error) {
	return nil, cloudprovider.ErrNotImplemented
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

func (r machineSetScalableResource) ID() string {
	return path.Join(r.Namespace(), r.Name())
}

func (r machineSetScalableResource) MaxSize() int {
	return r.maxSize
}

func (r machineSetScalableResource) MinSize() int {
	return r.minSize
}

func (r machineSetScalableResource) Name() string {
	return r.machineSet.Name
}

func (r machineSetScalableResource) Namespace() string {
	return r.machineSet.Namespace
}

func (r machineSetScalableResource) Nodes() ([]string, error) {
	return r.controller.machineSetNodeNames(r.machineSet)
}

func (r machineSetScalableResource) Replicas() int32 {
	return pointer.Int32PtrDerefOr(r.machineSet.Spec.Replicas, 0)
}

func (r machineSetScalableResource) SetSize(nreplicas int32) error {
	machineSet, err := r.clusterapiClient.MachineSets(r.Namespace()).Get(r.Name(), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get MachineSet %q: %v", r.ID(), err)
	}

	machineSet = machineSet.DeepCopy()
	machineSet.Spec.Replicas = &nreplicas

	_, err = r.clusterapiClient.MachineSets(r.Namespace()).Update(machineSet)
	if err != nil {
		return fmt.Errorf("unable to update number of replicas of machineset %q: %v", r.ID(), err)
	}
	return nil
}

func newMachineSetScalableResource(controller *machineController, machineSet *v1alpha1.MachineSet) (*machineSetScalableResource, error) {
	minSize, maxSize, err := parseScalingBounds(machineSet.Annotations)
	if err != nil {
		return nil, fmt.Errorf("error validating min/max annotations: %v", err)
	}

	return &machineSetScalableResource{
		clusterapiClient: controller.clusterClientset.ClusterV1alpha1(),
		controller:       controller,
		machineSet:       machineSet,
		maxSize:          maxSize,
		minSize:          minSize,
	}, nil
}

func (r machineDeploymentScalableResource) ID() string {
	return path.Join(r.Namespace(), r.Name())
}

func (r machineDeploymentScalableResource) MaxSize() int {
	return r.maxSize
}

func (r machineDeploymentScalableResource) MinSize() int {
	return r.minSize
}

func (r machineDeploymentScalableResource) Name() string {
	return r.machineDeployment.Name
}

func (r machineDeploymentScalableResource) Namespace() string {
	return r.machineDeployment.Namespace
}

func (r machineDeploymentScalableResource) Nodes() ([]string, error) {
	result := []string{}

	if err := r.controller.filterAllMachineSets(func(machineSet *v1alpha1.MachineSet) error {
		if machineSetIsOwnedByMachineDeployment(machineSet, r.machineDeployment) {
			names, err := r.controller.machineSetNodeNames(machineSet)
			if err != nil {
				return err
			}
			result = append(result, names...)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func (r machineDeploymentScalableResource) Replicas() int32 {
	return pointer.Int32PtrDerefOr(r.machineDeployment.Spec.Replicas, 0)
}

func (r machineDeploymentScalableResource) SetSize(nreplicas int32) error {
	machineDeployment, err := r.clusterapiClient.MachineDeployments(r.Namespace()).Get(r.Name(), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get MachineDeployment %q: %v", r.ID(), err)
	}

	machineDeployment = machineDeployment.DeepCopy()
	machineDeployment.Spec.Replicas = &nreplicas

	_, err = r.clusterapiClient.MachineDeployments(r.Namespace()).Update(machineDeployment)
	if err != nil {
		return fmt.Errorf("unable to update number of replicas of machineDeployment %q: %v", r.ID(), err)
	}
	return nil
}

func newMachineDeploymentScalableResource(controller *machineController, machineDeployment *v1alpha1.MachineDeployment) (*machineDeploymentScalableResource, error) {
	minSize, maxSize, err := parseScalingBounds(machineDeployment.Annotations)
	if err != nil {
		return nil, fmt.Errorf("error validating min/max annotations: %v", err)
	}

	return &machineDeploymentScalableResource{
		clusterapiClient:  controller.clusterClientset.ClusterV1alpha1(),
		controller:        controller,
		machineDeployment: machineDeployment,
		maxSize:           maxSize,
		minSize:           minSize,
	}, nil
}
