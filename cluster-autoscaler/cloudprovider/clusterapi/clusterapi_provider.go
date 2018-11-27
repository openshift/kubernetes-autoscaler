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

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/config"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	informers "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions"
)

const (
	// ProviderName is the name of cluster-api cloud provider.
	ProviderName = "cluster-api"
)

var _ cloudprovider.CloudProvider = (*provider)(nil)

type provider struct {
	*clusterController

	providerName    string
	resourceLimiter *cloudprovider.ResourceLimiter
	clusterapi      clusterv1alpha1.ClusterV1alpha1Interface
}

func (p *provider) nodeNames(machineSet *v1alpha1.MachineSet) ([]string, error) {
	machines, err := p.clusterController.machineInformer.Lister().Machines(machineSet.Namespace).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("error listing machines: %v", err)
	}

	var nodeNames []string

	for _, machine := range machines {
		if machine.Status.NodeRef == nil {
			glog.V(4).Infof("Status.NodeRef of machine %q is currently nil", machine.Name)
			continue
		}
		if machine.Status.NodeRef.Kind != "Node" {
			glog.Errorf("Status.NodeRef of machine %q does not reference a node (rather %q)", machine.Name, machine.Status.NodeRef.Kind)
			continue
		}
		if machineOwnerName(machine) == machineSet.Name {
			nodeNames = append(nodeNames, machine.Status.NodeRef.Name)
		}
	}

	glog.V(4).Infof("nodegroup %s has nodes %v", machineSet.Name, nodeNames)

	return nodeNames, nil
}

func (p *provider) nodeGroups() ([]*nodegroup, error) {
	var nodegroups []*nodegroup

	machineSets, err := p.clusterController.machineSetInformer.Lister().MachineSets("").List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, machineSet := range machineSets {
		nodegroup, err := p.buildNodeGroup(machineSet.DeepCopy())
		if err != nil {
			return nil, err
		}
		nodegroups = append(nodegroups, nodegroup)
	}

	return nodegroups, nil
}

func (p *provider) buildNodeGroup(machineSet *v1alpha1.MachineSet) (*nodegroup, error) {
	minSize, maxSize, err := parseMachineSetBounds(machineSet)

	if err != nil {
		return nil, fmt.Errorf("error validating min/max annotations: %v", err)
	}

	var replicas int32

	if machineSet.Spec.Replicas != nil {
		replicas = *machineSet.Spec.Replicas
	}

	nodeNames, err := p.nodeNames(machineSet)
	if err != nil {
		return nil, err
	}

	return &nodegroup{
		maxSize:   maxSize,
		minSize:   minSize,
		name:      machineSet.Name,
		namespace: machineSet.Namespace,
		nodeNames: nodeNames,
		provider:  p,
		replicas:  replicas,
	}, nil
}

func (p *provider) Name() string {
	return p.providerName
}

func (p *provider) GetResourceLimiter() (*cloudprovider.ResourceLimiter, error) {
	return p.resourceLimiter, nil
}

func (p *provider) NodeGroups() []cloudprovider.NodeGroup {
	nodegroups, err := p.nodeGroups()
	if err != nil {
		return nil
	}

	var result []cloudprovider.NodeGroup

	for _, ng := range nodegroups {
		info := fmt.Sprintf("min: %v, max: %v, replicas: %v", ng.minSize, ng.maxSize, ng.replicas)
		size := ng.MaxSize() - ng.MinSize()
		switch {
		case size > 0:
			result = append(result, ng)
			glog.V(4).Infof("discovered machineset %q (%q)", ng, info)
		case size < 0:
			glog.V(4).Infof("skipping machineset %q (%q): invalid min/max size(s)", ng, info)
		default:
			glog.V(4).Infof("skipping machineset %q (%q): max-min is zero", ng, info)
		}
	}

	return result
}

func (p *provider) NodeGroupForNode(node *apiv1.Node) (cloudprovider.NodeGroup, error) {
	// TODO(frobware) - currently waiting for:
	//
	//  https://github.com/kubernetes-sigs/cluster-api/pull/565
	//
	// to merge and then the logic here will switch to indexing
	// via node.Spec.ProviderID. What we have here uses a node's
	// "machine" annotation but that works for only a short period
	// of time as the overall core autoscaler logic uses
	// node.Spec.ProviderID to determine if a node is correctly
	// registered. As we use a node's "machine" annotation the
	// mapping this function provides between a node and its
	// corresponding nodegroup will eventually degrade to
	// "unregistered" and once that happens the autoscaler will
	// stop scaling up and down as it deems the overall state of
	// the cluster to be unhealthy.
	machine, err := p.clusterController.findMachine(node)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, nil
	}

	machineSet, err := p.clusterController.findMachineSet(machine)
	if err != nil {
		return nil, err
	}

	if machineSet == nil {
		return nil, nil
	}

	nodegroup, err := p.buildNodeGroup(machineSet)
	if err != nil {
		return nil, fmt.Errorf("failed to build nodegroup for node %q: %v", node.Name, err)
	}

	glog.V(4).Infof("node %q is in nodegroup %q", node.Name, machineSet.Name)
	return nodegroup, nil
}

func (*provider) Pricing() (cloudprovider.PricingModel, errors.AutoscalerError) {
	return nil, cloudprovider.ErrNotImplemented
}

func (*provider) GetAvailableMachineTypes() ([]string, error) {
	return []string{}, nil
}

func (*provider) NewNodeGroup(machineType string, labels map[string]string, systemLabels map[string]string, taints []apiv1.Taint, extraResources map[string]resource.Quantity) (cloudprovider.NodeGroup, error) {
	return nil, cloudprovider.ErrNotImplemented
}

func (*provider) Cleanup() error {
	return nil
}

func (p *provider) Refresh() error {
	return nil
}

// BuildCloudProvider builds a new clusterapi-based cloudprovider
func BuildCloudProvider(name string, opts config.AutoscalingOptions, rl *cloudprovider.ResourceLimiter) (cloudprovider.CloudProvider, error) {
	var err error
	var externalConfig *rest.Config

	externalConfig, err = rest.InClusterConfig()
	if err != nil && err != rest.ErrNotInCluster {
		return nil, err
	}

	if opts.KubeConfigPath != "" {
		externalConfig, err = clientcmd.BuildConfigFromFlags("", opts.KubeConfigPath)
		if err != nil {
			return nil, err
		}
	}

	clientset, err := clientset.NewForConfig(externalConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset failed: %v", err)
	}

	factory := informers.NewSharedInformerFactory(clientset, time.Second*30)
	controller, err := newClusterController(factory)
	if err != nil {
		return nil, err
	}

	// Ideally this would be passed in but the builder is not
	// currently organised to do so.
	stopCh := make(chan struct{})

	if err := controller.run(stopCh); err != nil {
		return nil, err
	}

	return &provider{
		providerName:      name,
		resourceLimiter:   rl,
		clusterController: controller,
		clusterapi:        clientset.ClusterV1alpha1(),
	}, nil
}
