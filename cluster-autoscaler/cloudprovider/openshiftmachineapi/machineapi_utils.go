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
	"strconv"

	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	nodeGroupMinSizeAnnotationKey = "machine.openshift.io/cluster-api-autoscaler-node-group-min-size"
	nodeGroupMaxSizeAnnotationKey = "machine.openshift.io/cluster-api-autoscaler-node-group-max-size"
)

var (
	// errMissingMinAnnotation is the error returned when a
	// machine set does not have an annotation keyed by
	// nodeGroupMinSizeAnnotationKey.
	errMissingMinAnnotation = errors.New("missing min annotation")

	// errMissingMaxAnnotation is the error returned when a
	// machine set does not have an annotation keyed by
	// nodeGroupMaxSizeAnnotationKey.
	errMissingMaxAnnotation = errors.New("missing max annotation")

	// errInvalidMinAnnotationValue is the error returned when a
	// machine set has a non-integral min annotation value.
	errInvalidMinAnnotation = errors.New("invalid min annotation")

	// errInvalidMaxAnnotationValue is the error returned when a
	// machine set has a non-integral max annotation value.
	errInvalidMaxAnnotation = errors.New("invalid max annotation")
)

// minSize returns the minimum value encoded in the annotations keyed
// by nodeGroupMinSizeAnnotationKey. Returns errMissingMinAnnotation
// if the annotation doesn't exist or errInvalidMinAnnotation if the
// value is not of type int.
func minSize(annotations map[string]string) (int, error) {
	val, found := annotations[nodeGroupMinSizeAnnotationKey]
	if !found {
		return 0, errMissingMinAnnotation
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0, errors.Wrapf(err, "%s", errInvalidMinAnnotation)
	}
	return i, nil
}

// maxSize returns the maximum value encoded in the annotations keyed
// by nodeGroupMaxSizeAnnotationKey. Returns errMissingMaxAnnotation
// if the annotation doesn't exist or errInvalidMaxAnnotation if the
// value is not of type int.
func maxSize(annotations map[string]string) (int, error) {
	val, found := annotations[nodeGroupMaxSizeAnnotationKey]
	if !found {
		return 0, errMissingMaxAnnotation
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0, errors.Wrapf(err, "%s", errInvalidMaxAnnotation)
	}
	return i, nil
}

func parseScalingBounds(annotations map[string]string) (int, int, error) {
	minSize, err := minSize(annotations)
	if err != nil && err != errMissingMinAnnotation {
		return 0, 0, err
	}

	if minSize < 0 {
		return 0, 0, errInvalidMinAnnotation
	}

	maxSize, err := maxSize(annotations)
	if err != nil && err != errMissingMaxAnnotation {
		return 0, 0, err
	}

	if maxSize < 0 {
		return 0, 0, errInvalidMaxAnnotation
	}

	if maxSize < minSize {
		return 0, 0, errInvalidMaxAnnotation
	}

	return minSize, maxSize, nil
}

func machineOwnerRef(machine *v1beta1.Machine) *metav1.OwnerReference {
	for _, ref := range machine.OwnerReferences {
		if ref.Kind == "MachineSet" && ref.Name != "" {
			return ref.DeepCopy()
		}
	}

	return nil
}

func machineIsOwnedByMachineSet(machine *v1beta1.Machine, machineSet *v1beta1.MachineSet) bool {
	if ref := machineOwnerRef(machine); ref != nil {
		return ref.UID == machineSet.UID
	}
	return false
}

func machineSetMachineDeploymentRef(machineSet *v1beta1.MachineSet) *metav1.OwnerReference {
	for _, ref := range machineSet.OwnerReferences {
		if ref.Kind == "MachineDeployment" {
			return ref.DeepCopy()
		}
	}

	return nil
}

func machineSetHasMachineDeploymentOwnerRef(machineSet *v1beta1.MachineSet) bool {
	return machineSetMachineDeploymentRef(machineSet) != nil
}

func machineSetIsOwnedByMachineDeployment(machineSet *v1beta1.MachineSet, machineDeployment *v1beta1.MachineDeployment) bool {
	if ref := machineSetMachineDeploymentRef(machineSet); ref != nil {
		return ref.UID == machineDeployment.UID
	}
	return false
}
