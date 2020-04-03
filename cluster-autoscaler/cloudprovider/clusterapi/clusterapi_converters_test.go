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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var ctTimestampNow = metav1.NewTime(time.Now().Truncate(time.Second))

const (
	ctAPIVersion  = "machine.openshift.io/v1beta1"
	ctKindMS      = "MachineSet"
	ctKindMD      = "MachineDeployment"
	ctKindOR      = "OwnerReference"
	ctName        = "SomeResource"
	ctNamespace   = "SomeNamespace"
	ctUID         = "XXXX"
	ctLabel1      = "label1"
	ctLabel1v     = "label1-value"
	ctTaintKey    = "taintKey"
	ctTaintValue  = "taintValue"
	ctTaintEffect = "taintEffect"
	ctReplicas    = 1
)

type converterTestConfig struct {
	name     string
	expected interface{}
	observed interface{}
	compare  func(interface{}, interface{}) bool
}

func testBoolCompare(expected interface{}, observed interface{}) bool {
	e := expected.(bool)
	o := observed.(bool)
	return (e == o)
}

func testInt32Compare(expected interface{}, observed interface{}) bool {
	e := expected.(int32)
	o := observed.(int32)
	return (e == o)
}

func testStringCompare(expected interface{}, observed interface{}) bool {
	e := expected.(string)
	o := observed.(string)
	return (e == o)
}

func testTimeCompare(expected interface{}, observed interface{}) bool {
	e := expected.(metav1.Time)
	o := observed.(metav1.Time)
	return (e == o)
}

func testObjectMeta() map[string]interface{} {
	return map[string]interface{}{
		"name":      ctName,
		"namespace": ctNamespace,
		"uid":       ctUID,
		"labels": map[string]interface{}{
			ctLabel1: ctLabel1v,
		},
		"annotations": map[string]interface{}{
			ctLabel1: ctLabel1v,
		},
		"ownerReferences": []interface{}{
			testOwnerReference(),
		},
		"deletionTimestamp": ctTimestampNow.ToUnstructured(),
	}
}

func testTaint() map[string]interface{} {
	return map[string]interface{}{
		"key":       ctTaintKey,
		"value":     ctTaintValue,
		"effect":    ctTaintEffect,
		"timeAdded": ctTimestampNow.ToUnstructured(),
	}
}

func testOwnerReference() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion":         ctAPIVersion,
		"kind":               ctKindOR,
		"name":               ctName,
		"uid":                ctUID,
		"controller":         true,
		"blockOwnerDeletion": true,
	}
}

func testMachineSetSpec() map[string]interface{} {
	return map[string]interface{}{
		"replicas": int64(ctReplicas),
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"metadata": testObjectMeta(),
				"taints": []interface{}{
					testTaint(),
				},
			},
		},
	}
}

func testMachineSet() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": ctAPIVersion,
		"kind":       ctKindMS,
		"metadata":   testObjectMeta(),
		"spec":       testMachineSetSpec(),
	}
}

func testMachineDeploymentSpec() map[string]interface{} {
	// for now we can return a machineset spec as it is functionally the same
	// as a machinedeploymentspec for these tests.
	return testMachineSetSpec()
}

func testMachineDeployment() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": ctAPIVersion,
		"kind":       ctKindMD,
		"metadata":   testObjectMeta(),
		"spec":       testMachineSetSpec(),
	}
}

func doCompares(configs []converterTestConfig, t *testing.T) {
	for _, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			if config.compare == nil {
				config.compare = testStringCompare
			}
			if !config.compare(config.expected, config.observed) {
				t.Errorf("%s improperly set. Expected %v, Observed %v", config.name, config.expected, config.observed)
			}
		})
	}
}

func TestConverterNewMachineSetFromUnstructured(t *testing.T) {
	testobj := unstructured.Unstructured{
		Object: testMachineSet(),
	}

	machineSet := newMachineSetFromUnstructured(&testobj)

	testConfigs := []converterTestConfig{
		{
			name:     "MachineSet.APIVersion",
			expected: ctAPIVersion,
			observed: machineSet.APIVersion,
		},
		{
			name:     "MachineSet.Kind",
			expected: ctKindMS,
			observed: machineSet.Kind,
		},
		{
			name:     "MachineSet.Name",
			expected: ctName,
			observed: machineSet.Name,
		},
		{
			name:     "machineSet.Namespace",
			expected: ctNamespace,
			observed: machineSet.Namespace,
		},
		{
			name:     "MachineSet.UID",
			expected: ctUID,
			observed: string(machineSet.UID),
		},
		{
			name:     "MachineSet.Labels",
			expected: ctLabel1v,
			observed: machineSet.Labels[ctLabel1],
		},
		{
			name:     "MachineSet.Annotations",
			expected: ctLabel1v,
			observed: machineSet.Annotations[ctLabel1],
		},
		{
			name:     "MachineSet.OwnerReferences.APIVersion",
			expected: ctAPIVersion,
			observed: machineSet.OwnerReferences[0].APIVersion,
		},
		{
			name:     "MachineSet.OwnerReferences.Kind",
			expected: ctKindOR,
			observed: machineSet.OwnerReferences[0].Kind,
		},
		{
			name:     "MachineSet.OwnerReferences.Name",
			expected: ctName,
			observed: machineSet.OwnerReferences[0].Name,
		},
		{
			name:     "MachineSet.OwnerReferences.UID",
			expected: ctUID,
			observed: string(machineSet.OwnerReferences[0].UID),
		},
		{
			name:     "MachineSet.OwnerReferences.Controller",
			expected: true,
			observed: *machineSet.OwnerReferences[0].Controller,
			compare:  testBoolCompare,
		},
		{
			name:     "MachineSet.OwnerReferences.BlockOwnerDeletion",
			expected: true,
			observed: *machineSet.OwnerReferences[0].BlockOwnerDeletion,
			compare:  testBoolCompare,
		},
		{
			name:     "MachineSet.DeletionTimestamp",
			expected: ctTimestampNow,
			observed: *machineSet.DeletionTimestamp,
			compare:  testTimeCompare,
		},
		{
			name:     "MachineSet.Spec.Replicas",
			expected: int32(ctReplicas),
			observed: *machineSet.Spec.Replicas,
			compare:  testInt32Compare,
		},
		{
			name:     "MachineSet.Spec.Template.Spec.Labels",
			expected: ctLabel1v,
			observed: machineSet.Spec.Template.Spec.Labels[ctLabel1],
		},
		{
			name:     "MachineSet.Spec.Template.Spec.Taints.Key",
			expected: ctTaintKey,
			observed: machineSet.Spec.Template.Spec.Taints[0].Key,
		},
		{
			name:     "MachineSet.Spec.Template.Spec.Taints.Value",
			expected: ctTaintValue,
			observed: machineSet.Spec.Template.Spec.Taints[0].Value,
		},
		{
			name:     "MachineSet.Spec.Template.Spec.Taints.Effect",
			expected: ctTaintEffect,
			observed: string(machineSet.Spec.Template.Spec.Taints[0].Effect),
		},
		{
			name:     "MachineSet.Spec.Template.Spec.Taints.TimeAdded",
			expected: metav1.NewTime(ctTimestampNow.UTC()),
			observed: *machineSet.Spec.Template.Spec.Taints[0].TimeAdded,
			compare:  testTimeCompare,
		},
	}

	doCompares(testConfigs, t)
}

func TestConverterNewMachineDeploymentFromUnstructured(t *testing.T) {
	testobj := unstructured.Unstructured{
		Object: testMachineDeployment(),
	}

	machineDeployment := newMachineDeploymentFromUnstructured(&testobj)

	testConfigs := []converterTestConfig{
		{
			name:     "MachineDeployment.APIVersion",
			expected: ctAPIVersion,
			observed: machineDeployment.APIVersion,
		},
		{
			name:     "MachineDeployment.Kind",
			expected: ctKindMD,
			observed: machineDeployment.Kind,
		},
		{
			name:     "MachineDeployment.Name",
			expected: ctName,
			observed: machineDeployment.Name,
		},
		{
			name:     "machineDeployment.Namespace",
			expected: ctNamespace,
			observed: machineDeployment.Namespace,
		},
		{
			name:     "MachineDeployment.UID",
			expected: ctUID,
			observed: string(machineDeployment.UID),
		},
		{
			name:     "MachineDeployment.Labels",
			expected: ctLabel1v,
			observed: machineDeployment.Labels[ctLabel1],
		},
		{
			name:     "MachineDeployment.Annotations",
			expected: ctLabel1v,
			observed: machineDeployment.Annotations[ctLabel1],
		},
		{
			name:     "MachineDeployment.OwnerReferences.APIVersion",
			expected: ctAPIVersion,
			observed: machineDeployment.OwnerReferences[0].APIVersion,
		},
		{
			name:     "MachineDeployment.OwnerReferences.Kind",
			expected: ctKindOR,
			observed: machineDeployment.OwnerReferences[0].Kind,
		},
		{
			name:     "MachineDeployment.OwnerReferences.Name",
			expected: ctName,
			observed: machineDeployment.OwnerReferences[0].Name,
		},
		{
			name:     "MachineDeployment.OwnerReferences.UID",
			expected: ctUID,
			observed: string(machineDeployment.OwnerReferences[0].UID),
		},
		{
			name:     "MachineDeployment.OwnerReferences.Controller",
			expected: true,
			observed: *machineDeployment.OwnerReferences[0].Controller,
			compare:  testBoolCompare,
		},
		{
			name:     "MachineDeployment.OwnerReferences.BlockOwnerDeletion",
			expected: true,
			observed: *machineDeployment.OwnerReferences[0].BlockOwnerDeletion,
			compare:  testBoolCompare,
		},
		{
			name:     "MachineDeployment.DeletionTimestamp",
			expected: ctTimestampNow,
			observed: *machineDeployment.DeletionTimestamp,
			compare:  testTimeCompare,
		},
		{
			name:     "MachineDeployment.Spec.Replicas",
			expected: int32(ctReplicas),
			observed: *machineDeployment.Spec.Replicas,
			compare:  testInt32Compare,
		},
		{
			name:     "MachineDeployment.Spec.Template.Spec.Labels",
			expected: ctLabel1v,
			observed: machineDeployment.Spec.Template.Spec.Labels[ctLabel1],
		},
		{
			name:     "MachineDeployment.Spec.Template.Spec.Taints.Key",
			expected: ctTaintKey,
			observed: machineDeployment.Spec.Template.Spec.Taints[0].Key,
		},
		{
			name:     "MachineDeployment.Spec.Template.Spec.Taints.Value",
			expected: ctTaintValue,
			observed: machineDeployment.Spec.Template.Spec.Taints[0].Value,
		},
		{
			name:     "MachineDeployment.Spec.Template.Spec.Taints.Effect",
			expected: ctTaintEffect,
			observed: string(machineDeployment.Spec.Template.Spec.Taints[0].Effect),
		},
		{
			name:     "MachineDeployment.Spec.Template.Spec.Taints.TimeAdded",
			expected: metav1.NewTime(ctTimestampNow.UTC()),
			observed: *machineDeployment.Spec.Template.Spec.Taints[0].TimeAdded,
			compare:  testTimeCompare,
		},
	}

	doCompares(testConfigs, t)
}
