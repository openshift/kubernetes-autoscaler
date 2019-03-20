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
	"os"

	"github.com/golang/glog"
	clusterclientset "github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/config"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// ProviderName is the name of cluster-api cloud provider.
	ProviderName = "openshift-machine-api"
)

var _ cloudprovider.CloudProvider = (*provider)(nil)

type provider struct {
	controller      *machineController
	providerName    string
	resourceLimiter *cloudprovider.ResourceLimiter
}

func (p *provider) Name() string {
	return p.providerName
}

func (p *provider) GetResourceLimiter() (*cloudprovider.ResourceLimiter, error) {
	return p.resourceLimiter, nil
}

func (p *provider) NodeGroups() []cloudprovider.NodeGroup {
	nodegroups, err := p.controller.nodeGroups()
	if err != nil {
		glog.Errorf("error getting node groups: %v", err)
		return nil
	}
	for _, ng := range nodegroups {
		glog.V(4).Infof("discovered node group: %s", ng.Debug())
	}
	return nodegroups
}

func (p *provider) NodeGroupForNode(node *apiv1.Node) (cloudprovider.NodeGroup, error) {
	return p.controller.nodeGroupForNode(node)
}

func (*provider) Pricing() (cloudprovider.PricingModel, errors.AutoscalerError) {
	return nil, cloudprovider.ErrNotImplemented
}

func (*provider) GetAvailableMachineTypes() ([]string, error) {
	return []string{}, nil
}

func (*provider) NewNodeGroup(
	machineType string,
	labels map[string]string,
	systemLabels map[string]string,
	taints []apiv1.Taint,
	extraResources map[string]resource.Quantity,
) (cloudprovider.NodeGroup, error) {
	return nil, cloudprovider.ErrNotImplemented
}

func (*provider) Cleanup() error {
	return nil
}

func (p *provider) Refresh() error {
	return nil
}

func (p *provider) GetInstanceID(node *apiv1.Node) string {
	return node.Spec.ProviderID
}

func newProvider(
	name string,
	rl *cloudprovider.ResourceLimiter,
	controller *machineController,
) (cloudprovider.CloudProvider, error) {
	return &provider{
		providerName:    name,
		resourceLimiter: rl,
		controller:      controller,
	}, nil
}

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

	kubeclient, err := kubernetes.NewForConfig(externalConfig)
	if err != nil {
		return nil, fmt.Errorf("create kube clientset failed: %v", err)
	}

	clusterclient, err := clusterclientset.NewForConfig(externalConfig)
	if err != nil {
		return nil, fmt.Errorf("create cluster clientset failed: %v", err)
	}

	enableMachineDeployments := os.Getenv("OPENSHIFT_MACHINE_API_CLOUDPROVIDER_ENABLE_MACHINE_DEPLOYMENTS") != ""
	controller, err := newMachineController(kubeclient, clusterclient, enableMachineDeployments)

	if err != nil {
		return nil, err
	}

	// Ideally this would be passed in but the builder is not
	// currently organised to do so.
	stopCh := make(chan struct{})

	if err := controller.run(stopCh); err != nil {
		return nil, err
	}

	return newProvider(name, rl, controller)
}
