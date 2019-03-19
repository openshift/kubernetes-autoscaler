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

package openshiftmachineapi

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	clusterclient "github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset"
	clusterinformers "github.com/openshift/cluster-api/pkg/client/informers_generated/externalversions"
	machinev1beta1 "github.com/openshift/cluster-api/pkg/client/informers_generated/externalversions/machine/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	kubeinformers "k8s.io/client-go/informers"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	nodeProviderIDIndex = "openshiftmachineapi-nodeProviderIDIndex"
)

// machineController watches for Nodes, Machines, MachineSets and
// MachineDeployments as they are added, updated and deleted on the
// cluster. Additionally, it adds indices to the node informers to
// satisfy lookup by node.Spec.ProviderID.
type machineController struct {
	clusterClientset          clusterclient.Interface
	clusterInformerFactory    clusterinformers.SharedInformerFactory
	kubeInformerFactory       kubeinformers.SharedInformerFactory
	machineDeploymentInformer machinev1beta1.MachineDeploymentInformer
	machineInformer           machinev1beta1.MachineInformer
	machineSetInformer        machinev1beta1.MachineSetInformer
	nodeInformer              cache.SharedIndexInformer
	enableMachineDeployments  bool
}

type machineSetFilterFunc func(machineSet *v1beta1.MachineSet) error

func indexNodeByNodeProviderID(obj interface{}) ([]string, error) {
	if node, ok := obj.(*apiv1.Node); ok {
		return []string{node.Spec.ProviderID}, nil
	}
	return []string{}, nil
}

func (c *machineController) findMachine(id string) (*v1beta1.Machine, error) {
	item, exists, err := c.machineInformer.Informer().GetStore().GetByKey(id)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	machine, ok := item.(*v1beta1.Machine)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", machine)
	}

	return machine.DeepCopy(), nil
}

func (c *machineController) findMachineDeployment(id string) (*v1beta1.MachineDeployment, error) {
	item, exists, err := c.machineDeploymentInformer.Informer().GetStore().GetByKey(id)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	machineDeployment, ok := item.(*v1beta1.MachineDeployment)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", machineDeployment)
	}

	return machineDeployment.DeepCopy(), nil
}

// findMachineOwner returns the machine set owner for machine, or nil
// if there is no owner. A DeepCopy() of the object is returned on
// success.
func (c *machineController) findMachineOwner(machine *v1beta1.Machine) (*v1beta1.MachineSet, error) {
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

	machineSet, ok := item.(*v1beta1.MachineSet)
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
		c.machineSetInformer.Informer().HasSynced,
		c.machineDeploymentInformer.Informer().HasSynced) {
		return fmt.Errorf("syncing caches failed")
	}

	return nil
}

// findMachineByNodeProviderID find associated machine using
// node.Spec.ProviderID as the key. Returns nil if either the Node by
// node.Spec.ProviderID cannot be found or if the node has no machine
// annotation. A DeepCopy() of the object is returned on success.
func (c *machineController) findMachineByNodeProviderID(node *apiv1.Node) (*v1beta1.Machine, error) {
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
	if machineName, found := node.Annotations["machine.openshift.io/machine"]; found {
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

// machinesInMachineSet returns all the machines that belong to
// machineSet. For each machine in the set a DeepCopy() of the object
// is returned.
func (c *machineController) machinesInMachineSet(machineSet *v1beta1.MachineSet) ([]*v1beta1.Machine, error) {
	listOptions := labels.SelectorFromSet(labels.Set(machineSet.Labels))
	machines, err := c.machineInformer.Lister().Machines(machineSet.Namespace).List(listOptions)
	if err != nil {
		return nil, err
	}

	var result []*v1beta1.Machine

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
	kubeclient kubeclient.Interface,
	clusterclient clusterclient.Interface,
	enableMachineDeployments bool,
) (*machineController, error) {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclient, 0)
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterclient, 0)

	machineInformer := clusterInformerFactory.Machine().V1beta1().Machines()
	machineSetInformer := clusterInformerFactory.Machine().V1beta1().MachineSets()
	machineDeploymentInformer := clusterInformerFactory.Machine().V1beta1().MachineDeployments()

	machineDeploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
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
		clusterClientset:          clusterclient,
		clusterInformerFactory:    clusterInformerFactory,
		kubeInformerFactory:       kubeInformerFactory,
		machineDeploymentInformer: machineDeploymentInformer,
		machineInformer:           machineInformer,
		machineSetInformer:        machineSetInformer,
		nodeInformer:              nodeInformer,
		enableMachineDeployments:  enableMachineDeployments,
	}, nil
}

func (c *machineController) machineSetNodeNames(machineSet *v1beta1.MachineSet) ([]string, error) {
	machines, err := c.machinesInMachineSet(machineSet)
	if err != nil {
		return nil, fmt.Errorf("error listing machines: %v", err)
	}

	var nodes []string

	for _, machine := range machines {
		if machine.Status.NodeRef == nil {
			glog.V(4).Infof("Status.NodeRef of machine %q is currently nil", machine.Name)
			continue
		}
		if machine.Status.NodeRef.Kind != "Node" {
			glog.Errorf("Status.NodeRef of machine %q does not reference a node (rather %q)", machine.Name, machine.Status.NodeRef.Kind)
			continue
		}

		node, err := c.findNodeByNodeName(machine.Status.NodeRef.Name)
		if err != nil {
			return nil, fmt.Errorf("unknown node %q", machine.Status.NodeRef.Name)
		}

		if node != nil {
			nodes = append(nodes, node.Spec.ProviderID)
		}
	}

	glog.V(4).Infof("nodegroup %s has nodes %v", machineSet.Name, nodes)

	return nodes, nil
}

func (c *machineController) filterAllMachineSets(f machineSetFilterFunc) error {
	return c.filterMachineSets(metav1.NamespaceAll, f)
}

func (c *machineController) filterMachineSets(namespace string, f machineSetFilterFunc) error {
	machineSets, err := c.machineSetInformer.Lister().MachineSets(namespace).List(labels.Everything())
	if err != nil {
		return nil
	}
	for _, machineSet := range machineSets {
		if err := f(machineSet); err != nil {
			return err
		}
	}
	return nil
}

func (c *machineController) machineSetNodeGroups() ([]cloudprovider.NodeGroup, error) {
	var nodegroups []cloudprovider.NodeGroup

	if err := c.filterAllMachineSets(func(machineSet *v1beta1.MachineSet) error {
		if machineSetHasMachineDeploymentOwnerRef(machineSet) {
			return nil
		}
		ng, err := newNodegroupFromMachineSet(c, machineSet.DeepCopy())
		if err != nil {
			return err
		}
		if ng.MaxSize()-ng.MinSize() > 0 {
			nodegroups = append(nodegroups, ng)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return nodegroups, nil
}

func (c *machineController) machineDeploymentNodeGroups() ([]cloudprovider.NodeGroup, error) {
	if !c.enableMachineDeployments {
		return nil, nil
	}

	machineDeployments, err := c.machineDeploymentInformer.Lister().MachineDeployments(apiv1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var nodegroups []cloudprovider.NodeGroup

	for _, md := range machineDeployments {
		ng, err := newNodegroupFromMachineDeployment(c, md.DeepCopy())
		if err != nil {
			return nil, err
		}
		// add nodegroup iff it has the capacity to scale
		if ng.MaxSize()-ng.MinSize() > 0 {
			nodegroups = append(nodegroups, ng)
		}
	}

	return nodegroups, nil
}

func (c *machineController) nodeGroups() ([]cloudprovider.NodeGroup, error) {
	machineSets, err := c.machineSetNodeGroups()
	if err != nil {
		return nil, err
	}

	machineDeployments, err := c.machineDeploymentNodeGroups()
	if err != nil {
		return nil, err
	}
	return append(machineSets, machineDeployments...), nil
}

func (c *machineController) nodeGroupForNode(node *apiv1.Node) (cloudprovider.NodeGroup, error) {
	machine, err := c.findMachineByNodeProviderID(node)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, nil
	}

	machineSet, err := c.findMachineOwner(machine)
	if err != nil {
		return nil, err
	}

	if machineSet == nil {
		return nil, nil
	}

	if c.enableMachineDeployments {
		if ref := machineSetMachineDeploymentRef(machineSet); ref != nil {
			key := fmt.Sprintf("%s/%s", machineSet.Namespace, ref.Name)
			machineDeployment, err := c.findMachineDeployment(key)
			if err != nil {
				return nil, fmt.Errorf("unknown MachineDeployment %q: %v", key, err)
			}
			if machineDeployment == nil {
				return nil, fmt.Errorf("unknown MachineDeployment %q", key)
			}
			nodegroup, err := newNodegroupFromMachineDeployment(c, machineDeployment)
			if err != nil {
				return nil, fmt.Errorf("failed to build nodegroup for node %q: %v", node.Name, err)
			}
			// We don't scale from 0 so nodes must belong
			// to a nodegroup that has a scale size of at
			// least 1.
			if nodegroup.MaxSize()-nodegroup.MinSize() < 1 {
				return nil, nil
			}
			return nodegroup, nil
		}
	}

	nodegroup, err := newNodegroupFromMachineSet(c, machineSet)
	if err != nil {
		return nil, fmt.Errorf("failed to build nodegroup for node %q: %v", node.Name, err)
	}

	// We don't scale from 0 so nodes must belong to a nodegroup
	// that has a scale size of at least 1.
	if nodegroup.MaxSize()-nodegroup.MinSize() < 1 {
		return nil, nil
	}

	glog.V(4).Infof("node %q is in nodegroup %q", node.Name, machineSet.Name)
	return nodegroup, nil
}
