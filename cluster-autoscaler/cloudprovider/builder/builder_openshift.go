//go:build openshift
// +build openshift

/*
Copyright 2026 The Kubernetes Authors.

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

package builder

import (
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/clusterapi"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/openshift"
	coreoptions "k8s.io/autoscaler/cluster-autoscaler/core/options"
	"k8s.io/client-go/informers"
)

// AvailableCloudProviders supported by the cloud provider builder.
var AvailableCloudProviders = []string{
	cloudprovider.ClusterAPIProviderName,
	cloudprovider.OpenShiftProviderName,
}

// DefaultCloudProvider for OpenShift-only build is OpenShift.
const DefaultCloudProvider = cloudprovider.OpenShiftProviderName

func buildCloudProvider(opts *coreoptions.AutoscalerOptions, do cloudprovider.NodeGroupDiscoveryOptions, rl *cloudprovider.ResourceLimiter, _ informers.SharedInformerFactory) cloudprovider.CloudProvider {
	switch opts.CloudProviderName {
	case cloudprovider.ClusterAPIProviderName:
		return clusterapi.BuildClusterAPI(opts, do, rl)
	case cloudprovider.OpenShiftProviderName:
		return openshift.BuildOpenShift(opts, do, rl)
	}

	return nil
}
