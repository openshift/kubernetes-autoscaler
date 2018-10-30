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
	"time"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	schedulercache "k8s.io/kubernetes/pkg/scheduler/cache"
)

const (
	machineDeleteAnnotationKey = "sigs.k8s.io/cluster-api-delete-machine"
)

var _ cloudprovider.NodeGroup = (*nodegroup)(nil)

type nodegroup struct {
	maxSize   int
	minSize   int
	name      string
	namespace string
	nodeNames []string
	provider  *provider
	replicas  int32
}

func (ng *nodegroup) Name() string {
	return ng.name
}

func (ng *nodegroup) Namespace() string {
	return ng.namespace
}

func (ng *nodegroup) MinSize() int {
	return ng.minSize
}

func (ng *nodegroup) MaxSize() int {
	return ng.maxSize
}

func (ng *nodegroup) Replicas() int {
	return int(ng.replicas)
}

func (ng *nodegroup) SetSize(nreplicas int) error {
	machineSet, err := ng.provider.clusterapi.MachineSets(ng.namespace).Get(ng.name, v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get machineset %q: %v", ng.name, err)
	}

	machineSet = machineSet.DeepCopy()
	replicas := int32(nreplicas)
	machineSet.Spec.Replicas = &replicas

	_, err = ng.provider.clusterapi.MachineSets(ng.namespace).Update(machineSet)
	if err != nil {
		return fmt.Errorf("unable to update number of replicas of machineset %q: %v", ng, err)
	}
	return nil
}

func (ng *nodegroup) String() string {
	return fmt.Sprintf("%s/%s", ng.namespace, ng.name)
}

// TargetSize returns the current target size of the node group. It is
// possible that the number of nodes in Kubernetes is different at the
// moment but should be equal to Size() once everything stabilizes
// (new nodes finish startup and registration or removed nodes are
// deleted completely). Implementation required.
func (ng *nodegroup) TargetSize() (int, error) {
	return ng.Replicas(), nil
}

// IncreaseSize increases the size of the node group. To delete a node
// you need to explicitly name it and use DeleteNode. This function
// should wait until node group size is updated. Implementation
// required.
func (ng *nodegroup) IncreaseSize(delta int) error {
	if delta <= 0 {
		return fmt.Errorf("size increase must be positive")
	}
	size := ng.Replicas()
	if size+delta > ng.MaxSize() {
		return fmt.Errorf("size increase too large - desired:%d max:%d", size+delta, ng.MaxSize())
	}
	return ng.SetSize(size + delta)
}

// DeleteNodes deletes nodes from this node group. Error is returned
// either on failure or if the given node doesn't belong to this node
// group. This function should wait until node group size is updated.
// Implementation required.
func (ng *nodegroup) DeleteNodes(nodes []*apiv1.Node) error {
	for _, node := range nodes {
		machine, err := ng.provider.clusterController.findMachine(node)
		if err != nil {
			return err
		}
		if machine == nil {
			return fmt.Errorf("unknown machine")
		}

		machine = machine.DeepCopy()

		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}

		machine.Annotations[machineDeleteAnnotationKey] = time.Now().String()

		if _, err := ng.provider.clusterapi.Machines(machine.Namespace).Update(machine); err != nil {
			return fmt.Errorf("failed to update machine %s/%s: %v", machine.Namespace, machine.Name, err)
		}
	}

	replicas := ng.Replicas()
	if replicas-len(nodes) <= 0 {
		return fmt.Errorf("unable to delete %d machines in %s, machine replicas are <= 0 ", len(nodes), ng.name)
	}

	return ng.SetSize(replicas - len(nodes))
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

	return ng.SetSize(size + delta)
}

// Id returns an unique identifier of the node group.
func (ng *nodegroup) Id() string {
	return ng.name
}

// Debug returns a string containing all information regarding this node group.
func (ng *nodegroup) Debug() string {
	return fmt.Sprintf("%s (min: %d, max: %d, replicas: %d)", ng.Id(), ng.MinSize(), ng.MaxSize(), ng.Replicas())
}

// Nodes returns a list of all nodes that belong to this node group.
func (ng *nodegroup) Nodes() ([]string, error) {
	return ng.nodeNames, nil
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
