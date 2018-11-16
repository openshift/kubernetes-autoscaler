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
	"strconv"
	"time"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	schedulercache "k8s.io/kubernetes/pkg/scheduler/cache"
	v1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	machineDeleteAnnotationKey    = "sigs.k8s.io/cluster-api-delete-machine"
	nodeGroupMinSizeAnnotationKey = "sigs.k8s.io/cluster-api-autoscaler-node-group-min-size"
	nodeGroupMaxSizeAnnotationKey = "sigs.k8s.io/cluster-api-autoscaler-node-group-max-size"
)

var _ cloudprovider.NodeGroup = (*nodegroup)(nil)

type nodegroup struct {
	*provider
	*v1alpha1.MachineSet
	minSize int
	maxSize int
	nodes   []string
}

func (ng *nodegroup) Name() string {
	return ng.MachineSet.Name
}

func (ng *nodegroup) Namespace() string {
	return ng.MachineSet.Namespace
}

func (ng *nodegroup) MinSize() int {
	return ng.minSize
}

func (ng *nodegroup) MaxSize() int {
	return ng.maxSize
}

func (ng *nodegroup) Replicas() int {
	if ng.MachineSet.Spec.Replicas == nil {
		return 0
	}
	glog.Infof("machineset: %q has %d replicas", ng.MachineSet.Name, *ng.MachineSet.Spec.Replicas)
	return int(*ng.MachineSet.Spec.Replicas)
}

func (ng *nodegroup) SetSize(nreplicas int) error {
	ms, err := ng.clusterapi.MachineSets(ng.MachineSet.Namespace).Get(ng.MachineSet.Name, v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Unable to get machineset %q: %v", ng.MachineSet.Name, err)
	}

	newMachineSet := ms.DeepCopy()
	replicas := int32(nreplicas)
	newMachineSet.Spec.Replicas = &replicas

	_, err = ng.clusterapi.MachineSets(ng.MachineSet.Namespace).Update(newMachineSet)
	if err != nil {
		return fmt.Errorf("Unable to update number of replicas of machineset %q: %v", ng.MachineSet.Name, err)
	}

	return nil
}

func (ng *nodegroup) String() string {
	return fmt.Sprintf("%s/%s", ng.Namespace(), ng.Name())
}

func parseAnnotation(ms *v1alpha1.MachineSet, key string) (int, error) {
	val, exists := ms.Annotations[key]
	if !exists {
		glog.Infof("machineset %q has no annotation for %q", machineSetID(ms), key)
		return 0, nil
	}

	u, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		// Returns "... cannot parse annotation <key> as an integral value: strconv.ParseUint: parsing "<val>": invalid syntax"
		return 0, fmt.Errorf("machineset %q: cannot parse annotation %q as an integral value: %v", machineSetID(ms), key, err)
	}

	return int(u), nil
}

func newClusterMachineSet(m *provider, ms *v1alpha1.MachineSet, nodes []string) (*nodegroup, error) {
	cms := nodegroup{
		provider:   m,
		MachineSet: ms,
		nodes:      nodes,
	}

	minSize, err := parseAnnotation(ms, nodeGroupMinSizeAnnotationKey)
	if err != nil {
		return nil, err
	}

	maxSize, err := parseAnnotation(ms, nodeGroupMaxSizeAnnotationKey)
	if err != nil {
		return nil, err
	}

	if maxSize < minSize {
		return nil, fmt.Errorf("machineset %q: max value (%q:%d) must be >= min value (%q:%d)",
			machineSetID(ms),
			nodeGroupMaxSizeAnnotationKey, maxSize,
			nodeGroupMinSizeAnnotationKey, minSize)
	}

	cms.minSize = minSize
	cms.maxSize = maxSize

	return &cms, nil
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
	if len(nodes) == 0 {
		return nil
	}

	snapshot := ng.getClusterState()

	for _, node := range nodes {
		msid := machineSetID(ng.MachineSet)
		nodemap, exists := snapshot.NodeMap[msid]
		if !exists {
			return fmt.Errorf("unknown machineset %q", msid)
		}

		name, exists := nodemap[node.Name]
		if !exists {
			return fmt.Errorf("cannot map node %q to machine", node.Name)
		}
		machine, err := ng.clusterapi.Machines(ng.MachineSet.Namespace).Get(name, v1.GetOptions{})
		if err != nil {
			return fmt.Errorf("cannot get machine %s/%s: %v", ng.MachineSet.Namespace, name, err)
		}

		machine = machine.DeepCopy()

		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}

		machine.Annotations[machineDeleteAnnotationKey] = time.Now().String()

		_, err = ng.clusterapi.Machines(ng.MachineSet.Namespace).Update(machine)
		if err != nil {
			return fmt.Errorf("unable to update machine %q: %v", machine.Name, err)
		}
	}

	replicas := ng.Replicas()
	if replicas-len(nodes) <= 0 {
		return fmt.Errorf("unable to delete %d machines in %s, machine replicas are <= 0 ", len(nodes), ng.Name())
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

	if int(size)+delta < len(nodes) {
		return fmt.Errorf("attempt to delete existing nodes targetSize:%d delta:%d existingNodes: %d",
			size, delta, len(nodes))
	}

	return ng.SetSize(size + delta)
}

// Id returns an unique identifier of the node group.
func (ng *nodegroup) Id() string {
	return ng.Name()
}

// Debug returns a string containing all information regarding this node group.
func (ng *nodegroup) Debug() string {
	return fmt.Sprintf("%s (min: %d, max: %d, replicas: %d)", ng.Id(), ng.MinSize(), ng.MaxSize(), ng.Replicas())
}

// Nodes returns a list of all nodes that belong to this node group.
func (ng *nodegroup) Nodes() ([]string, error) {
	return ng.nodes, nil
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
