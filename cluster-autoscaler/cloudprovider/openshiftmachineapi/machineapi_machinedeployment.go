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

type machineDeploymentScalableResource struct {
	machineapiClient machinev1beta1.MachineV1beta1Interface
	controller       *machineController
	name             string
	namespace        string
	maxSize          int
	minSize          int
	replicas         int32
}

var _ scalableResource = (*machineDeploymentScalableResource)(nil)

func (r *machineDeploymentScalableResource) ID() string {
	return path.Join(r.namespace, r.name)
}

func (r *machineDeploymentScalableResource) MaxSize() int {
	return r.maxSize
}

func (r *machineDeploymentScalableResource) MinSize() int {
	return r.minSize
}

func (r *machineDeploymentScalableResource) Name() string {
	return r.name
}

func (r *machineDeploymentScalableResource) Namespace() string {
	return r.namespace
}

func (r *machineDeploymentScalableResource) Nodes() ([]string, error) {
	machineDeployment, err := r.machineapiClient.MachineDeployments(r.Namespace()).Get(r.Name(), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to get MachineDeployment %q: %v", r.ID(), err)
	}

	result := []string{}

	if err := r.controller.filterAllMachineSets(func(machineSet *v1beta1.MachineSet) error {
		if machineSetIsOwnedByMachineDeployment(machineSet, machineDeployment) {
			names, err := r.controller.machineSetNodeNames(machineSet)
			if err != nil {
				return err
			}
			result = append(result, names...)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *machineDeploymentScalableResource) TargetSize() (int32, error) {
	return r.replicas, nil
}

func (r *machineDeploymentScalableResource) SetSize(nreplicas int32) error {
	machineDeployment, err := r.machineapiClient.MachineDeployments(r.Namespace()).Get(r.Name(), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get MachineDeployment %q: %v", r.ID(), err)
	}

	machineDeployment = machineDeployment.DeepCopy()
	machineDeployment.Spec.Replicas = &nreplicas

	_, err = r.machineapiClient.MachineDeployments(r.Namespace()).Update(machineDeployment)
	if err != nil {
		return fmt.Errorf("unable to update number of replicas of machineDeployment %q: %v", r.ID(), err)
	}

	r.replicas = pointer.Int32PtrDerefOr(machineDeployment.Spec.Replicas, 0)
	return nil
}

func newMachineDeploymentScalableResource(controller *machineController, machineDeployment *v1beta1.MachineDeployment) (*machineDeploymentScalableResource, error) {
	minSize, maxSize, err := parseScalingBounds(machineDeployment.Annotations)
	if err != nil {
		return nil, fmt.Errorf("error validating min/max annotations: %v", err)
	}

	return &machineDeploymentScalableResource{
		machineapiClient: controller.clusterClientset.MachineV1beta1(),
		controller:       controller,
		name:             machineDeployment.Name,
		namespace:        machineDeployment.Namespace,
		maxSize:          maxSize,
		minSize:          minSize,
		replicas:         pointer.Int32PtrDerefOr(machineDeployment.Spec.Replicas, 0),
	}, nil
}
