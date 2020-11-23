/*
Copyright 2020 The Kubernetes Authors.

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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/autoscaler/cluster-autoscaler/utils/units"
)

const (
	nodeGroupMinSizeAnnotationKey = "machine.openshift.io/cluster-api-autoscaler-node-group-min-size"
	nodeGroupMaxSizeAnnotationKey = "machine.openshift.io/cluster-api-autoscaler-node-group-max-size"

	cpuKey     = "machine.openshift.io/vCPU"
	memoryKey  = "machine.openshift.io/memoryMb"
	gpuKey     = "machine.openshift.io/GPU"
	maxPodsKey = "machine.openshift.io/maxPods"
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

	zeroQuantity = resource.MustParse("0")
)

type normalizedProviderID string

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

func getOwnerForKind(u *unstructured.Unstructured, kind string) *metav1.OwnerReference {
	for _, ref := range u.GetOwnerReferences() {
		if ref.Kind == kind && ref.Name != "" {
			return ref.DeepCopy()
		}
	}

	return nil
}

func machineOwnerRef(machine *unstructured.Unstructured) *metav1.OwnerReference {
	return getOwnerForKind(machine, machineSetKind)
}

func machineSetOwnerRef(machineSet *unstructured.Unstructured) *metav1.OwnerReference {
	return getOwnerForKind(machineSet, machineDeploymentKind)
}

func machineSetHasMachineDeploymentOwnerRef(machineSet *unstructured.Unstructured) bool {
	return machineSetOwnerRef(machineSet) != nil
}

// normalizedProviderString splits s on '/' returning everything after
// the last '/'.
func normalizedProviderString(s string) normalizedProviderID {
	split := strings.Split(s, "/")
	return normalizedProviderID(split[len(split)-1])
}

func scaleFromZeroEnabled(annotations map[string]string) bool {
	cpu := annotations[cpuKey]
	mem := annotations[memoryKey]

	if cpu != "" && mem != "" {
		return true
	}
	return false
}

func parseKey(annotations map[string]string, key string) (resource.Quantity, error) {
	if val, exists := annotations[key]; exists && val != "" {
		return resource.ParseQuantity(val)
	}
	return zeroQuantity.DeepCopy(), nil
}

func parseCPUCapacity(annotations map[string]string) (resource.Quantity, error) {
	return parseKey(annotations, cpuKey)
}

func parseMemoryCapacity(annotations map[string]string) (resource.Quantity, error) {
	// The value for the memoryKey is expected to be an integer representing Mebibytes. e.g. "1024".
	// https://www.iec.ch/si/binary.htm
	val, exists := annotations[memoryKey]
	if exists && val != "" {
		valInt, err := strconv.ParseInt(val, 10, 0)
		if err != nil {
			return zeroQuantity.DeepCopy(), fmt.Errorf("value %q from annotation %q expected to be an integer: %v", val, memoryKey, err)
		}
		// Convert from Mebibytes to bytes
		return *resource.NewQuantity(valInt*units.MiB, resource.DecimalSI), nil
	}
	return zeroQuantity.DeepCopy(), nil
}

func parseGPUCapacity(annotations map[string]string) (resource.Quantity, error) {
	return parseKey(annotations, gpuKey)
}

func parseMaxPodsCapacity(annotations map[string]string) (resource.Quantity, error) {
	return parseKey(annotations, maxPodsKey)
}
