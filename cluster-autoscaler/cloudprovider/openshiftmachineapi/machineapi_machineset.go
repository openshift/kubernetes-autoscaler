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
	"path"

	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	machinev1beta1 "github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset/typed/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

type machineSetScalableResource struct {
	machineapiClient machinev1beta1.MachineV1beta1Interface
	controller       *machineController
	machineSet       *v1beta1.MachineSet
	maxSize          int
	minSize          int
}

var _ scalableResource = (*machineSetScalableResource)(nil)

func (r machineSetScalableResource) ID() string {
	return path.Join(r.Namespace(), r.Name())
}

func (r machineSetScalableResource) MaxSize() int {
	return r.maxSize
}

func (r machineSetScalableResource) MinSize() int {
	return r.minSize
}

func (r machineSetScalableResource) Name() string {
	return r.machineSet.Name
}

func (r machineSetScalableResource) Namespace() string {
	return r.machineSet.Namespace
}

func (r machineSetScalableResource) Nodes() ([]string, error) {
	return r.controller.machineSetProviderIDs(r.machineSet)
}

func (r machineSetScalableResource) Replicas() int32 {
	return pointer.Int32PtrDerefOr(r.machineSet.Spec.Replicas, 0)
}

func (r machineSetScalableResource) SetSize(nreplicas int32) error {
	machineSet, err := r.machineapiClient.MachineSets(r.Namespace()).Get(r.Name(), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get MachineSet %q: %v", r.ID(), err)
	}

	machineSet = machineSet.DeepCopy()
	machineSet.Spec.Replicas = &nreplicas

	_, err = r.machineapiClient.MachineSets(r.Namespace()).Update(machineSet)
	if err != nil {
		return fmt.Errorf("unable to update number of replicas of machineset %q: %v", r.ID(), err)
	}
	return nil
}

func newMachineSetScalableResource(controller *machineController, machineSet *v1beta1.MachineSet) (*machineSetScalableResource, error) {
	minSize, maxSize, err := parseScalingBounds(machineSet.Annotations)
	if err != nil {
		return nil, fmt.Errorf("error validating min/max annotations: %v", err)
	}

	return &machineSetScalableResource{
		machineapiClient: controller.clusterClientset.MachineV1beta1(),
		controller:       controller,
		machineSet:       machineSet,
		maxSize:          maxSize,
		minSize:          minSize,
	}, nil
}
