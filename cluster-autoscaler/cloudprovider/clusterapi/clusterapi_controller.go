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

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterinformers "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions/cluster/v1alpha1"
)

const (
	nodeProviderIDIndex = "clusterapi-nodeProviderIDIndex"
)

// machineController watches for Nodes, Machines and MachineSets as
// they are added, updated and deleted on the cluster. Additionally,
// it adds indices to the node informers to satisfy lookup by
// node.Spec.ProviderID.
type machineController struct {
	clusterInformerFactory clusterinformers.SharedInformerFactory
	kubeInformerFactory    kubeinformers.SharedInformerFactory
	machineInformer        clusterv1alpha1.MachineInformer
	machineSetInformer     clusterv1alpha1.MachineSetInformer
	nodeInformer           cache.SharedIndexInformer
}

func indexNodeByNodeProviderID(obj interface{}) ([]string, error) {
	if node, ok := obj.(*apiv1.Node); ok {
		return []string{node.Spec.ProviderID}, nil
	}
	return []string{}, nil
}

func (c *machineController) findMachine(id string) (*v1alpha1.Machine, error) {
	item, exists, err := c.machineInformer.Informer().GetStore().GetByKey(id)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	machine, ok := item.(*v1alpha1.Machine)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", machine)
	}

	return machine.DeepCopy(), nil
}

// findMachineOwner returns the machine set owner for machine, or nil
// if there is no owner. A DeepCopy() of the object is returned on
// success.
func (c *machineController) findMachineOwner(machine *v1alpha1.Machine) (*v1alpha1.MachineSet, error) {
	machineOwnerRef := machineOwnerRef(machine)
	if machineOwnerRef == nil {
		return nil, nil
	}

	store := c.machineSetInformer.Informer().GetStore()
	item, exists, err := store.GetByKey(fmt.Sprintf("%s/%s", machine.Namespace, machineOwnerRef.Name))
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	machineSet, ok := item.(*v1alpha1.MachineSet)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type: %T", machineSet)
	}

	if !machineIsOwnedByMachineSet(machine, machineSet) {
		return nil, nil
	}

	return machineSet.DeepCopy(), nil
}

// run starts shared informers and waits for the informer cache to
// synchronize.
func (c *machineController) run(stopCh <-chan struct{}) error {
	c.kubeInformerFactory.Start(stopCh)
	c.clusterInformerFactory.Start(stopCh)

	glog.V(4).Infof("waiting for caches to sync")
	if !cache.WaitForCacheSync(stopCh,
		c.nodeInformer.HasSynced,
		c.machineInformer.Informer().HasSynced,
		c.machineSetInformer.Informer().HasSynced) {
		return fmt.Errorf("syncing caches failed")
	}

	return nil
}

// findMachineByNodeProviderID find associated machine using
// node.Spec.ProviderID as the key. Returns nil if either the Node by
// node.Spec.ProviderID cannot be found or if the node has no machine
// annotation. A DeepCopy() of the object is returned on success.
func (c *machineController) findMachineByNodeProviderID(node *apiv1.Node) (*v1alpha1.Machine, error) {
	objs, err := c.nodeInformer.GetIndexer().ByIndex(nodeProviderIDIndex, node.Spec.ProviderID)
	if err != nil {
		return nil, err
	}

	switch n := len(objs); {
	case n == 0:
		return nil, nil
	case n > 1:
		return nil, fmt.Errorf("internal error; expected len==1, got %v", n)
	}

	node, ok := objs[0].(*apiv1.Node)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", node)
	}

	// TODO(frobware)
	//
	// Reference this annotation key symbolically once the
	// following PR merges:
	//     https://github.com/kubernetes-sigs/cluster-api/pull/663
	if machineName, found := node.Annotations["cluster.k8s.io/machine"]; found {
		return c.findMachine(machineName)
	}

	return nil, nil
}

// findNodeByNodeName find the Node object keyed by node.Name. Returns
// nil if it cannot be found. A DeepCopy() of the object is returned
// on success.
func (c *machineController) findNodeByNodeName(name string) (*apiv1.Node, error) {
	item, exists, err := c.nodeInformer.GetIndexer().GetByKey(name)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	node, ok := item.(*apiv1.Node)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", node)
	}

	return node.DeepCopy(), nil
}

// MachinesInMachineSet returns all the machines that belong to
// machineSet. For each machine in the set a DeepCopy() of the object
// is returned.
func (c *machineController) MachinesInMachineSet(machineSet *v1alpha1.MachineSet) ([]*v1alpha1.Machine, error) {
	listOptions := labels.SelectorFromSet(labels.Set(machineSet.Labels))
	machines, err := c.machineInformer.Lister().Machines(machineSet.Namespace).List(listOptions)
	if err != nil {
		return nil, err
	}

	var result []*v1alpha1.Machine

	for _, machine := range machines {
		if machineIsOwnedByMachineSet(machine, machineSet) {
			result = append(result, machine.DeepCopy())
		}
	}

	return result, nil
}

// newMachineController constructs a controller that watches Nodes,
// Machines and MachineSet as they are added, updated and deleted on
// the cluster.
func newMachineController(
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	clusterInformerFactory clusterinformers.SharedInformerFactory,
) (*machineController, error) {
	machineInformer := clusterInformerFactory.Cluster().V1alpha1().Machines()
	machineSetInformer := clusterInformerFactory.Cluster().V1alpha1().MachineSets()

	machineInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	machineSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})

	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})

	indexerFuncs := cache.Indexers{
		nodeProviderIDIndex: indexNodeByNodeProviderID,
	}

	if err := nodeInformer.GetIndexer().AddIndexers(indexerFuncs); err != nil {
		return nil, fmt.Errorf("cannot add indexers: %v", err)
	}

	return &machineController{
		clusterInformerFactory: clusterInformerFactory,
		kubeInformerFactory:    kubeInformerFactory,
		machineInformer:        machineInformer,
		machineSetInformer:     machineSetInformer,
		nodeInformer:           nodeInformer,
	}, nil
}
