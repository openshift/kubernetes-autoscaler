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
	"strconv"

	"github.com/golang/glog"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	nodeGroupMinSizeAnnotationKey = "sigs.k8s.io/cluster-api-autoscaler-node-group-min-size"
	nodeGroupMaxSizeAnnotationKey = "sigs.k8s.io/cluster-api-autoscaler-node-group-max-size"
)

func parseMachineSetAnnotation(machineSet *v1alpha1.MachineSet, key string) (int, error) {
	val, found := machineSet.Annotations[key]
	if !found {
		glog.V(4).Infof("machineset %s/%s has no annotation %q", machineSet.Namespace, machineSet.Name, key)
		return 0, nil
	}

	u, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		return 0, err
	}

	return int(u), nil
}

func parseMachineSetBounds(machineSet *v1alpha1.MachineSet) (int, int, error) {
	minSize, err := parseMachineSetAnnotation(machineSet, nodeGroupMinSizeAnnotationKey)
	if err != nil {
		return 0, 0, err
	}

	maxSize, err := parseMachineSetAnnotation(machineSet, nodeGroupMaxSizeAnnotationKey)
	if err != nil {
		return 0, 0, err
	}

	if maxSize < minSize {
		return 0, 0, fmt.Errorf("max value (%q:%d) must be >= min value (%q:%d)",
			nodeGroupMaxSizeAnnotationKey, maxSize,
			nodeGroupMinSizeAnnotationKey, minSize)
	}

	return minSize, maxSize, nil
}

func machineOwnerName(m *v1alpha1.Machine) string {
	for _, ref := range m.OwnerReferences {
		if ref.Kind == "MachineSet" && ref.Name != "" {
			return ref.Name
		}
	}

	return ""
}
