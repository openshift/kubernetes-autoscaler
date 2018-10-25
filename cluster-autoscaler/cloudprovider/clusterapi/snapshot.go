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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1alpha1apis "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type MachineSetID string

type clusterSnapshot struct {
	NodeMap            map[MachineSetID]map[string]string
	MachineSetMap      map[MachineSetID]*nodegroup
	NodeToMachineSetID map[string]MachineSetID
	MachineSetNodeMap  map[MachineSetID][]string
}

func machineSetID(m *v1alpha1apis.MachineSet) MachineSetID {
	return MachineSetID(fmt.Sprintf("%s/%s", m.Namespace, m.Name))
}

func getMachinesInMachineSet(p *provider, ms *v1alpha1apis.MachineSet) ([]*v1alpha1apis.Machine, error) {
	machines, err := p.clusterapi.Machines(ms.Namespace).List(v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(ms.Spec.Selector.MatchLabels).String(),
	})

	if err != nil {
		return nil, fmt.Errorf("unable to list machines in namespace %s: %v", ms.Namespace, err)
	}

	names := make([]string, len(machines.Items))
	result := make([]*v1alpha1apis.Machine, len(machines.Items))

	for i := range machines.Items {
		names[i] = machines.Items[i].Name
		result[i] = &machines.Items[i]
	}

	glog.Infof("%d machines in machineset %s/%s: %#v", len(result), ms.Namespace, ms.Name, names)

	return result, nil
}

func getMachineSetsInNamespace(p *provider, namespace string) ([]*v1alpha1apis.MachineSet, error) {
	machineSets, err := p.clusterapi.MachineSets(namespace).List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to list machinesets in namespace %q: %v", namespace, err)
	}

	names := make([]string, len(machineSets.Items))
	result := make([]*v1alpha1apis.MachineSet, len(machineSets.Items))

	for i := range machineSets.Items {
		names[i] = machineSets.Items[i].Name
		result[i] = &machineSets.Items[i]
	}

	return result, nil
}

func getNamespaces(p *provider) ([]string, error) {
	namespaces, err := p.kubeclient.CoreV1().Namespaces().List(v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]string, len(namespaces.Items))

	for i := range namespaces.Items {
		result[i] = namespaces.Items[i].Name
	}

	return result, nil
}

func mapMachinesForMachineSet(p *provider, snapshot *clusterSnapshot, ms *v1alpha1apis.MachineSet) error {
	machines, err := getMachinesInMachineSet(p, ms)
	if err != nil {
		return err
	}

	msid := machineSetID(ms)

	snapshot.NodeMap[msid] = make(map[string]string)

	for _, machine := range machines {
		if machine.Status.NodeRef == nil {
			glog.Errorf("Status.NodeRef of machine %q is nil", machine.Name)
			continue
		}
		if machine.Status.NodeRef.Kind != "Node" {
			glog.Errorf("Status.NodeRef of machine %q does not reference a node (rather %q)", machine.Name, machine.Status.NodeRef.Kind)
			continue
		}
		snapshot.NodeMap[msid][machine.Status.NodeRef.Name] = machine.Name
		snapshot.NodeToMachineSetID[machine.Status.NodeRef.Name] = msid
		snapshot.MachineSetNodeMap[msid] = append(snapshot.MachineSetNodeMap[msid], machine.Status.NodeRef.Name)
	}

	return nil
}

func mapMachineSetsForNS(p *provider, snapshot *clusterSnapshot, namespace string) error {
	machineSets, err := getMachineSetsInNamespace(p, namespace)
	if err != nil {
		return err
	}

	for _, ms := range machineSets {
		if err := mapMachinesForMachineSet(p, snapshot, ms); err != nil {
			return err
		}
		msid := machineSetID(ms)
		cms, err := newClusterMachineSet(p, ms, snapshot.MachineSetNodeMap[msid])
		if err != nil {
			return err
		}
		snapshot.MachineSetMap[msid] = cms
	}

	return nil
}

func getClusterSnapshot(p *provider) (*clusterSnapshot, error) {
	snapshot := newEmptySnapshot()
	namespaces, err := getNamespaces(p)
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaces {
		if err := mapMachineSetsForNS(p, snapshot, ns); err != nil {
			return nil, err
		}
	}

	return snapshot, err
}

func newEmptySnapshot() *clusterSnapshot {
	return &clusterSnapshot{
		NodeMap:            make(map[MachineSetID]map[string]string),
		MachineSetMap:      make(map[MachineSetID]*nodegroup),
		MachineSetNodeMap:  make(map[MachineSetID][]string),
		NodeToMachineSetID: make(map[string]MachineSetID),
	}
}
