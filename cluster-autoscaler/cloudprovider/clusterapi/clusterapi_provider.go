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
	"sync"
	"time"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/config"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	v1alpha1apis "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

const (
	// ProviderName is the cloud nodegroup name for the cluster-api
	// nodegroup.
	ProviderName = "cluster-api"

	refreshInterval = 30 * time.Second
)

var _ cloudprovider.CloudProvider = (*provider)(nil)

type provider struct {
	// protects the cluster snapshot
	clusterSnapshotMutex sync.Mutex
	clusterSnapshot      *clusterSnapshot

	providerName    string
	resourceLimiter *cloudprovider.ResourceLimiter
	lastRefresh     time.Time
	clusterapi      v1alpha1apis.ClusterV1alpha1Interface
	kubeclient      *kubeclient.Clientset
}

func (p *provider) Name() string {
	return p.providerName
}

func (p *provider) GetResourceLimiter() (*cloudprovider.ResourceLimiter, error) {
	return p.resourceLimiter, nil
}

func (p *provider) NodeGroups() []cloudprovider.NodeGroup {
	result := []cloudprovider.NodeGroup{}

	for _, ms := range p.getClusterState().MachineSetMap {
		if ms.MaxSize()-ms.MinSize() > 0 {
			result = append(result, ms)
		}
	}

	return result
}

func (p *provider) NodeGroupForNode(node *apiv1.Node) (cloudprovider.NodeGroup, error) {
	snapshot := p.getClusterState()
	if msid, exists := snapshot.NodeToMachineSetID[node.Name]; exists {
		return snapshot.MachineSetMap[msid], nil
	}
	return nil, nil
}

func (p *provider) getClusterState() *clusterSnapshot {
	p.clusterSnapshotMutex.Lock()
	defer p.clusterSnapshotMutex.Unlock()
	return p.clusterSnapshot
}

func (p *provider) setClusterState(s *clusterSnapshot) {
	p.clusterSnapshotMutex.Lock()
	defer p.clusterSnapshotMutex.Unlock()
	p.clusterSnapshot = s
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

func (c *provider) Refresh() error {
	if c.lastRefresh.Add(refreshInterval).After(time.Now()) && c.clusterSnapshot != nil {
		return nil
	}

	s, err := getClusterSnapshot(c)
	if err == nil {
		c.lastRefresh = time.Now()
		c.setClusterState(s)
	}

	return err
}

func NewProvider(name string, opts config.AutoscalingOptions, do cloudprovider.NodeGroupDiscoveryOptions, rl *cloudprovider.ResourceLimiter) (*provider, error) {
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

	kubeclient, err := kubeclient.NewForConfig(externalConfig)
	if err != nil {
		return nil, err
	}

	clusterapi, err := clientset.NewForConfig(externalConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create client for talking to the apiserver: %v", err)
	}

	return &provider{
		clusterSnapshot: newEmptySnapshot(),
		providerName:    name,
		resourceLimiter: rl,
		clusterapi:      clusterapi.ClusterV1alpha1(),
		kubeclient:      kubeclient,
	}, nil
}
