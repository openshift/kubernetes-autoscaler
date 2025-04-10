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
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	gpuapis "k8s.io/autoscaler/cluster-autoscaler/utils/gpu"
	klog "k8s.io/klog/v2"
)

type unstructuredScalableResource struct {
	controller         *machineController
	unstructured       *unstructured.Unstructured
	maxSize            int
	minSize            int
	autoscalingOptions map[string]string
}

func (r unstructuredScalableResource) ID() string {
	return path.Join(r.Kind(), r.Namespace(), r.Name())
}

func (r unstructuredScalableResource) MaxSize() int {
	return r.maxSize
}

func (r unstructuredScalableResource) MinSize() int {
	return r.minSize
}

func (r unstructuredScalableResource) Kind() string {
	return r.unstructured.GetKind()
}

func (r unstructuredScalableResource) GroupVersionResource() (schema.GroupVersionResource, error) {
	switch r.Kind() {
	case machineDeploymentKind:
		return r.controller.machineDeploymentResource, nil
	case machineSetKind:
		return r.controller.machineSetResource, nil
	case machinePoolKind:
		return r.controller.machinePoolResource, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unknown scalable resource kind %s", r.Kind())
	}
}

func (r unstructuredScalableResource) Name() string {
	return r.unstructured.GetName()
}

func (r unstructuredScalableResource) Namespace() string {
	return r.unstructured.GetNamespace()
}

func (r unstructuredScalableResource) ProviderIDs() ([]string, error) {
	providerIds, err := r.controller.scalableResourceProviderIDs(r.unstructured)
	if err != nil {
		return nil, err
	}

	return providerIds, nil
}

func (r unstructuredScalableResource) Replicas() (int, error) {
	gvr, err := r.GroupVersionResource()
	if err != nil {
		return 0, err
	}

	// this function needs to differentiate between machine-api and cluster-api
	// due to the fact that the machine-api controllers exclude machines in
	// deleting phase when calculating replicas.
	if gvr.Group == openshiftMAPIGroup {
		return r.replicasOpenshift()
	}

	s, err := r.controller.managementScaleClient.Scales(r.Namespace()).Get(context.TODO(), gvr.GroupResource(), r.Name(), metav1.GetOptions{})
	if err != nil {
		return 0, err
	}

	if s == nil {
		return 0, fmt.Errorf("failed to fetch resource scale: unknown %s %s/%s", r.Kind(), r.Namespace(), r.Name())
	}
	return int(s.Spec.Replicas), nil
}

func (r unstructuredScalableResource) replicasOpenshift() (int, error) {
	gvr, err := r.GroupVersionResource()
	if err != nil {
		return 0, fmt.Errorf("error getting GVR in replicasOpenshift: %w", err)
	}

	if gvr.Group != openshiftMAPIGroup {
		return 0, fmt.Errorf("incorrect group for replica count on %s %s/%s", r.Kind(), r.Namespace(), r.Name())
	}

	// get the selector labels from the scalable resource to find the machines
	rawSelector, found, err := unstructured.NestedMap(r.unstructured.Object, "spec", "selector")
	if !found || err != nil {
		return 0, fmt.Errorf("error getting selector in replicasOpenshift: %w", err)
	}

	// we want to massage the unstructured selector data into a LabelSelector struct
	// so that we can more easily create the necessary string for the ListOptions struct,
	// the following code helps with that.
	data, err := json.Marshal(rawSelector)
	if err != nil {
		return 0, fmt.Errorf("error marshaling selector in replicasOpenshift: %w", err)
	}

	var labelSelector metav1.LabelSelector
	if err := json.Unmarshal(data, &labelSelector); err != nil {
		return 0, fmt.Errorf("error unmarshaling selector in replicasOpenshift: %w", err)
	}

	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return 0, fmt.Errorf("error seting label selector in replicasOpenshift: %w", err)
	}

	// get a list of machines filtered by the namespace and the selector labels from the scalable resource
	machinesList, err := r.controller.managementClient.Resource(r.controller.machineResource).Namespace(r.Namespace()).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return 0, fmt.Errorf("error listing machines in replicasOpenshift: %w", err)
	}

	// filter out inactive machines
	var activeMachines []unstructured.Unstructured
	for _, item := range machinesList.Items {
		if metav1.GetControllerOf(&item) != nil && !metav1.IsControlledBy(&item, r.unstructured) {
			continue
		}

		activeMachines = append(activeMachines, item)
	}

	return len(activeMachines), nil
}

func (r unstructuredScalableResource) SetSize(nreplicas int) error {
	switch {
	case nreplicas > r.maxSize:
		return fmt.Errorf("size increase too large - desired:%d max:%d", nreplicas, r.maxSize)
	case nreplicas < r.minSize:
		return fmt.Errorf("size decrease too large - desired:%d min:%d", nreplicas, r.minSize)
	}

	gvr, err := r.GroupVersionResource()
	if err != nil {
		return err
	}

	s, err := r.controller.managementScaleClient.Scales(r.Namespace()).Get(context.TODO(), gvr.GroupResource(), r.Name(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	if s == nil {
		return fmt.Errorf("unknown %s %s/%s", r.Kind(), r.Namespace(), r.Name())
	}

	s.Spec.Replicas = int32(nreplicas)
	_, updateErr := r.controller.managementScaleClient.Scales(r.Namespace()).Update(context.TODO(), gvr.GroupResource(), s, metav1.UpdateOptions{})

	if updateErr == nil {
		updateErr = unstructured.SetNestedField(r.unstructured.UnstructuredContent(), int64(nreplicas), "spec", "replicas")
	}

	return updateErr
}

func (r unstructuredScalableResource) UnmarkMachineForDeletion(machine *unstructured.Unstructured) error {
	u, err := r.controller.managementClient.Resource(r.controller.machineResource).Namespace(machine.GetNamespace()).Get(context.TODO(), machine.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	annotations := u.GetAnnotations()
	delete(annotations, machineDeleteAnnotationKey)
	delete(annotations, oldMachineDeleteAnnotationKey)
	u.SetAnnotations(annotations)
	_, updateErr := r.controller.managementClient.Resource(r.controller.machineResource).Namespace(u.GetNamespace()).Update(context.TODO(), u, metav1.UpdateOptions{})

	return updateErr
}

func (r unstructuredScalableResource) MarkMachineForDeletion(machine *unstructured.Unstructured) error {
	u, err := r.controller.managementClient.Resource(r.controller.machineResource).Namespace(machine.GetNamespace()).Get(context.TODO(), machine.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	u = u.DeepCopy()

	annotations := u.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations[machineDeleteAnnotationKey] = time.Now().String()
	annotations[oldMachineDeleteAnnotationKey] = time.Now().String()
	u.SetAnnotations(annotations)

	_, updateErr := r.controller.managementClient.Resource(r.controller.machineResource).Namespace(u.GetNamespace()).Update(context.TODO(), u, metav1.UpdateOptions{})

	return updateErr
}

func (r unstructuredScalableResource) Labels() map[string]string {
	labels := make(map[string]string, 0)

	newlabels, found, err := unstructured.NestedStringMap(r.unstructured.Object, "spec", "template", "spec", "metadata", "labels")
	if err != nil {
		return nil
	}
	if found {
		for k, v := range newlabels {
			labels[k] = v
		}
	}

	annotations := r.unstructured.GetAnnotations()
	// annotation value of the form "key1=value1,key2=value2"
	if val, found := annotations[labelsKey]; found {
		newlabels := strings.Split(val, ",")
		for _, label := range newlabels {
			split := strings.SplitN(label, "=", 2)
			if len(split) == 2 {
				labels[split[0]] = split[1]
			}
		}
	}

	return labels
}

func (r unstructuredScalableResource) Taints() []apiv1.Taint {
	taints := make([]apiv1.Taint, 0)

	newtaints, found, err := unstructured.NestedSlice(r.unstructured.Object, "spec", "template", "spec", "taints")
	if err != nil {
		return nil
	}
	if found {
		for _, t := range newtaints {
			if t := unstructuredToTaint(t); t != nil {
				taints = append(taints, *t)
			} else {
				klog.Warningf("Unable to convert of type %T data to taint: %+v", t, t)
				continue
			}
		}
	}

	annotations := r.unstructured.GetAnnotations()
	// annotation value the form of "key1=value1:condition,key2=value2:condition"
	if val, found := annotations[taintsKey]; found {
		newtaints := strings.Split(val, ",")
		for _, taintStr := range newtaints {
			taint, err := parseTaint(taintStr)
			if err == nil {
				taints = append(taints, taint)
			}
		}
	}

	return taints
}

func unstructuredToTaint(unstructuredTaintInterface interface{}) *corev1.Taint {
	unstructuredTaint := unstructuredTaintInterface.(map[string]interface{})
	if unstructuredTaint == nil {
		return nil
	}

	taint := &corev1.Taint{}
	taint.Key = unstructuredTaint["key"].(string)
	// value is optional and could be nil if not present
	if unstructuredTaint["value"] != nil {
		taint.Value = unstructuredTaint["value"].(string)
	}
	taint.Effect = corev1.TaintEffect(unstructuredTaint["effect"].(string))
	return taint
}

// A node group can scale from zero if it can inform about the CPU and memory
// capacity of the nodes within the group.
func (r unstructuredScalableResource) CanScaleFromZero() bool {
	capacity, err := r.InstanceCapacity()
	if err != nil {
		return false
	}
	// CPU and memory are the minimum necessary for scaling from zero
	_, cpuOk := capacity[corev1.ResourceCPU]
	_, memOk := capacity[corev1.ResourceMemory]

	return cpuOk && memOk
}

// Inspect the annotations on the scalable resource, and the status.capacity
// field of the machine template infrastructure resource to build the projected
// capacity for this node group. The returned map will be empty if the
// provider does not support scaling from zero, or the annotations have not
// been added.
func (r unstructuredScalableResource) InstanceCapacity() (map[corev1.ResourceName]resource.Quantity, error) {
	capacityAnnotations := map[corev1.ResourceName]resource.Quantity{}

	cpu, err := r.InstanceCPUCapacityAnnotation()
	if err != nil {
		return nil, err
	}
	if !cpu.IsZero() {
		capacityAnnotations[corev1.ResourceCPU] = cpu
	}

	mem, err := r.InstanceMemoryCapacityAnnotation()
	if err != nil {
		return nil, err
	}
	if !mem.IsZero() {
		capacityAnnotations[corev1.ResourceMemory] = mem
	}

	disk, err := r.InstanceEphemeralDiskCapacityAnnotation()
	if err != nil {
		return nil, err
	}
	if !disk.IsZero() {
		capacityAnnotations[corev1.ResourceEphemeralStorage] = disk
	}

	gpuCount, err := r.InstanceGPUCapacityAnnotation()
	if err != nil {
		return nil, err
	}
	if !gpuCount.IsZero() {
		// OpenShift does not yet use the gpu-type annotation, and assumes nvidia gpu
		capacityAnnotations[gpuapis.ResourceNvidiaGPU] = gpuCount
	}

	maxPods, err := r.InstanceMaxPodsCapacityAnnotation()
	if err != nil {
		return nil, err
	}
	if maxPods.IsZero() {
		maxPods = *resource.NewQuantity(defaultMaxPods, resource.DecimalSI)
	}
	capacityAnnotations[corev1.ResourcePods] = maxPods

	infraObj, err := r.readInfrastructureReferenceResource()
	if err != nil || infraObj == nil {
		// because it is possible that the infrastructure provider does not implement
		// the capacity in the infrastructure reference, if there are annotations we
		// should return them here.
		// Check against 1 here because the max pods is always set.
		if len(capacityAnnotations) > 1 {
			return capacityAnnotations, nil
		}
		return nil, err
	}
	capacityInfraStatus := resourceCapacityFromInfrastructureObject(infraObj)

	// The annotations should override any values from the status block of the machine template.
	// We loop through the status block capacity first, then overwrite any values with the
	// annotation capacities.
	capacity := map[corev1.ResourceName]resource.Quantity{}
	for k, v := range capacityInfraStatus {
		capacity[k] = v
	}
	for k, v := range capacityAnnotations {
		capacity[k] = v
	}

	return capacity, nil
}

func (r unstructuredScalableResource) InstanceEphemeralDiskCapacityAnnotation() (resource.Quantity, error) {
	return parseEphemeralDiskCapacity(r.unstructured.GetAnnotations())
}

func (r unstructuredScalableResource) InstanceCPUCapacityAnnotation() (resource.Quantity, error) {
	return parseCPUCapacity(r.unstructured.GetAnnotations())
}

func (r unstructuredScalableResource) InstanceMemoryCapacityAnnotation() (resource.Quantity, error) {
	return parseMemoryCapacity(r.unstructured.GetAnnotations())
}

func (r unstructuredScalableResource) InstanceGPUCapacityAnnotation() (resource.Quantity, error) {
	return parseGPUCount(r.unstructured.GetAnnotations())
}

func (r unstructuredScalableResource) InstanceGPUTypeAnnotation() string {
	return parseGPUType(r.unstructured.GetAnnotations())
}

func (r unstructuredScalableResource) InstanceMaxPodsCapacityAnnotation() (resource.Quantity, error) {
	return parseMaxPodsCapacity(r.unstructured.GetAnnotations())
}

func (r unstructuredScalableResource) readInfrastructureReferenceResource() (*unstructured.Unstructured, error) {
	infraref, found, err := unstructured.NestedStringMap(r.unstructured.Object, "spec", "template", "spec", "infrastructureRef")
	if !found || err != nil {
		return nil, nil
	}

	apiversion, ok := infraref["apiVersion"]
	if !ok {
		return nil, nil
	}
	kind, ok := infraref["kind"]
	if !ok {
		return nil, nil
	}
	name, ok := infraref["name"]
	if !ok {
		return nil, nil
	}
	// kind needs to be lower case and plural
	kind = fmt.Sprintf("%ss", strings.ToLower(kind))
	gvk := schema.FromAPIVersionAndKind(apiversion, kind)
	res := schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: gvk.Kind}

	infra, err := r.controller.getInfrastructureResource(res, name, r.Namespace())
	if err != nil {
		klog.V(4).Infof("Unable to read infrastructure reference, error: %v", err)
		return nil, err
	}

	return infra, nil
}

func newUnstructuredScalableResource(controller *machineController, u *unstructured.Unstructured) (*unstructuredScalableResource, error) {
	annotations := u.GetAnnotations()
	minSize, maxSize, err := parseScalingBounds(annotations)
	if err != nil {
		return nil, errors.Wrap(err, "error validating min/max annotations")
	}

	return &unstructuredScalableResource{
		controller:         controller,
		unstructured:       u,
		maxSize:            maxSize,
		minSize:            minSize,
		autoscalingOptions: autoscalingOptions(annotations),
	}, nil
}

func resourceCapacityFromInfrastructureObject(infraobj *unstructured.Unstructured) map[corev1.ResourceName]resource.Quantity {
	capacity := map[corev1.ResourceName]resource.Quantity{}

	infracap, found, err := unstructured.NestedStringMap(infraobj.Object, "status", "capacity")
	if !found || err != nil {
		return capacity
	}

	for k, v := range infracap {
		// if we cannot parse the quantity, don't add it to the capacity
		if value, err := resource.ParseQuantity(v); err == nil {
			capacity[corev1.ResourceName(k)] = value
		}
	}

	return capacity
}

// adapted from https://github.com/kubernetes/kubernetes/blob/release-1.25/pkg/util/taints/taints.go#L39
func parseTaint(st string) (apiv1.Taint, error) {
	var taint apiv1.Taint

	var key string
	var value string
	var effect apiv1.TaintEffect

	parts := strings.Split(st, ":")
	switch len(parts) {
	case 1:
		key = parts[0]
	case 2:
		effect = apiv1.TaintEffect(parts[1])

		partsKV := strings.Split(parts[0], "=")
		if len(partsKV) > 2 {
			return taint, fmt.Errorf("invalid taint spec: %v", st)
		}
		key = partsKV[0]
		if len(partsKV) == 2 {
			value = partsKV[1]
			if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
				return taint, fmt.Errorf("invalid taint spec: %v, %s", st, strings.Join(errs, "; "))
			}
		}
	default:
		return taint, fmt.Errorf("invalid taint spec: %v", st)
	}

	if errs := validation.IsQualifiedName(key); len(errs) > 0 {
		return taint, fmt.Errorf("invalid taint spec: %v, %s", st, strings.Join(errs, "; "))
	}

	taint.Key = key
	taint.Value = value
	taint.Effect = effect

	return taint, nil
}
