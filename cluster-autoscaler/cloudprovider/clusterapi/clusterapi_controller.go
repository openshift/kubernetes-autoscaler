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
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	informers "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions/cluster/v1alpha1"
)

const (
	nodeNameIndexKey = "clusterapi-nodeNameIndex"
)

// clusterController watches for Machines and MachineSets as they are
// added, updated and deleted on the cluster.
type clusterController struct {
	informerFactory    informers.SharedInformerFactory
	machineInformer    clusterv1alpha1.MachineInformer
	machineSetInformer clusterv1alpha1.MachineSetInformer
}

func indexMachineByNodeName(obj interface{}) ([]string, error) {
	machine, ok := obj.(*v1alpha1.Machine)
	if !ok {
		return []string{}, nil
	}

	if machine.Status.NodeRef == nil || machine.Status.NodeRef.Kind != "Node" {
		return []string{}, nil
	}

	glog.V(4).Infof("machine %s/%s is node %q", machine.Namespace, machine.Name, machine.Status.NodeRef.Name)

	return []string{machine.Status.NodeRef.Name}, nil
}

func (c *clusterController) findMachine(node *apiv1.Node) (*v1alpha1.Machine, error) {
	machineIndexer := c.machineInformer.Informer().GetIndexer()
	objs, err := machineIndexer.ByIndex(nodeNameIndexKey, node.Name)
	if err != nil {
		return nil, err
	}
	if len(objs) != 1 {
		return nil, nil
	}

	machine, ok := objs[0].(*v1alpha1.Machine)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type: %T", machine)
	}

	return machine.DeepCopy(), nil
}

func (c *clusterController) findMachineSet(machine *v1alpha1.Machine) (*v1alpha1.MachineSet, error) {
	machineSetName := machineOwnerName(machine)
	if machineSetName == "" {
		return nil, nil
	}

	store := c.machineSetInformer.Informer().GetStore()
	item, exists, err := store.GetByKey(fmt.Sprintf("%s/%s", machine.Namespace, machineSetName))
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

	return machineSet.DeepCopy(), nil
}

// run starts shared informers and waits for the shared informer cache
// to synchronize.
func (c *clusterController) run(stopCh <-chan struct{}) error {
	c.informerFactory.Start(stopCh)

	glog.V(4).Infof("waiting for machine cache to sync")
	if !cache.WaitForCacheSync(stopCh, c.machineInformer.Informer().HasSynced) {
		return fmt.Errorf("cannot sync machine cache")
	}

	glog.V(4).Infof("waiting for machineset cache to sync")
	if !cache.WaitForCacheSync(stopCh, c.machineSetInformer.Informer().HasSynced) {
		return fmt.Errorf("cannot sync machineset cache")
	}

	return nil
}

func newClusterController(factory informers.SharedInformerFactory) (*clusterController, error) {
	machineInformer := factory.Cluster().V1alpha1().Machines()
	machineSetInformer := factory.Cluster().V1alpha1().MachineSets()

	machineInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	machineSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})

	indexerFuncs := cache.Indexers{
		nodeNameIndexKey: indexMachineByNodeName,
	}

	if err := machineInformer.Informer().GetIndexer().AddIndexers(indexerFuncs); err != nil {
		return nil, fmt.Errorf("cannot add indexers to machineInformer: %v", err)
	}

	return &clusterController{
		informerFactory:    factory,
		machineInformer:    machineInformer,
		machineSetInformer: machineSetInformer,
	}, nil
}
