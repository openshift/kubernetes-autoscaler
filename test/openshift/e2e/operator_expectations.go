package main

import (
	"strings"
	"time"

	"context"
	"fmt"

	"github.com/golang/glog"
	mapiv1beta1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	caov1alpha1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1alpha1"
	kappsapi "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	waitShort  = 1 * time.Minute
	waitMedium = 3 * time.Minute
	waitLong   = 10 * time.Minute
)

func (tc *testConfig) ExpectOperatorAvailable() error {
	name := "machine-api-operator"
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	d := &kappsapi.Deployment{}

	err := wait.PollImmediate(1*time.Second, waitShort, func() (bool, error) {
		if err := tc.client.Get(context.TODO(), key, d); err != nil {
			glog.Errorf("error querying api for Deployment object: %v, retrying...", err)
			return false, nil
		}
		if d.Status.ReadyReplicas < 1 {
			return false, nil
		}
		return true, nil
	})
	return err
}

// ExpectAutoscalerScalesOut is an smoke test for the autoscaling feature
// Create a clusterAutoscaler object
// Create a machineAutoscaler object
// Create a workLoad to force autoscaling
// Validate the targeted machineSet scales out the field for the expected number of replicas
// Validate the number of nodes in the cluster is growing
// Delete the workLoad
// Delete the autoscaler object
// Ensure initial number of replicas and nodes
func (tc *testConfig) ExpectAutoscalerScalesOut() error {
	listOptions := client.ListOptions{
		Namespace: namespace,
	}
	glog.Info("Get one machineSet")
	machineSetList := mapiv1beta1.MachineSetList{}
	err := wait.PollImmediate(1*time.Second, waitMedium, func() (bool, error) {
		if err := tc.client.List(context.TODO(), &listOptions, &machineSetList); err != nil {
			glog.Errorf("error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		return len(machineSetList.Items) > 0, nil
	})
	if err != nil {
		return err
	}

	// When we add support for machineDeployments on the installer, cluster-autoscaler and cluster-autoscaler-operator
	// we need to test against deployments instead so we skip this test.
	targetMachineSet := machineSetList.Items[0]
	if ownerReferences := targetMachineSet.GetOwnerReferences(); len(ownerReferences) > 0 {
		glog.Infof("MachineSet %s is owned by a machineDeployment. Please run tests agains machineDeployment instead", targetMachineSet.Name)
		return nil
	}

	glog.Infof("Create ClusterAutoscaler and MachineAutoscaler objects. Targeting machineSet %s", targetMachineSet.Name)
	initialNumberOfReplicas := targetMachineSet.Spec.Replicas
	clusterAutoscaler := caov1alpha1.ClusterAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterAutoscaler",
			APIVersion: "autoscaling.openshift.io/v1alpha1",
		},
	}
	machineAutoscaler := caov1alpha1.MachineAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("autoscale-%s", targetMachineSet.Name),
			Namespace:    namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineAutoscaler",
			APIVersion: "autoscaling.openshift.io/v1alpha1",
		},
		Spec: caov1alpha1.MachineAutoscalerSpec{
			MaxReplicas: 12,
			MinReplicas: 1,
			ScaleTargetRef: caov1alpha1.CrossVersionObjectReference{
				Name:       targetMachineSet.Name,
				Kind:       "MachineSet",
				APIVersion: "machine.openshift.io/v1beta1",
			},
		},
	}
	err = wait.PollImmediate(1*time.Second, waitMedium, func() (bool, error) {
		if err := tc.client.Create(context.TODO(), &clusterAutoscaler); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				glog.Errorf("error querying api for clusterAutoscaler object: %v, retrying...", err)
				return false, nil
			}
		}
		if err := tc.client.Create(context.TODO(), &machineAutoscaler); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				glog.Errorf("error querying api for machineAutoscaler object: %v, retrying...", err)
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	glog.Info("Get nodeList")
	nodeList := corev1.NodeList{}
	err = wait.PollImmediate(1*time.Second, waitMedium, func() (bool, error) {
		if err := tc.client.List(context.TODO(), &listOptions, &nodeList); err != nil {
			glog.Errorf("error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	clusterInitialTotalNodes := len(nodeList.Items)
	glog.Infof("Cluster initial number of nodes is %d", clusterInitialTotalNodes)

	glog.Info("Create workload")
	mem, err := resource.ParseQuantity("500Mi")
	if err != nil {
		glog.Fatalf("failed to ParseQuantity %v", err)
	}
	cpu, err := resource.ParseQuantity("500m")
	if err != nil {
		glog.Fatalf("failed to ParseQuantity %v", err)
	}
	backoffLimit := int32(4)
	completions := int32(50)
	parallelism := int32(50)
	activeDeadlineSeconds := int64(100)
	workLoad := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workload",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "workload",
							Image: "busybox",
							Command: []string{
								"sleep",
								"300",
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"memory": mem,
									"cpu":    cpu,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicy("Never"),
				},
			},
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			BackoffLimit:          &backoffLimit,
			Completions:           &completions,
			Parallelism:           &parallelism,
		},
	}
	err = wait.PollImmediate(1*time.Second, waitMedium, func() (bool, error) {
		if err := tc.client.Create(context.TODO(), &workLoad); err != nil {
			glog.Errorf("error querying api for workLoad object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	glog.Info("Wait for cluster to scale out number of replicas")
	err = wait.PollImmediate(1*time.Second, waitLong, func() (bool, error) {
		msKey := types.NamespacedName{
			Namespace: namespace,
			Name:      targetMachineSet.Name,
		}
		ms := &mapiv1beta1.MachineSet{}
		if err := tc.client.Get(context.TODO(), msKey, ms); err != nil {
			glog.Errorf("error querying api for clusterAutoscaler object: %v, retrying...", err)
			return false, nil
		}
		glog.Infof("MachineSet %s. Initial number of replicas: %d. New number of replicas: %d", targetMachineSet.Name, *initialNumberOfReplicas, *ms.Spec.Replicas)
		return *ms.Spec.Replicas > *initialNumberOfReplicas, nil
	})
	if err != nil {
		return err
	}

	glog.Info("Wait for cluster to scale out nodes")
	err = wait.PollImmediate(1*time.Second, waitLong, func() (bool, error) {
		nodeList := corev1.NodeList{}
		if err := tc.client.List(context.TODO(), &listOptions, &nodeList); err != nil {
			glog.Errorf("error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		glog.Info("Expect at least a new node to come up")
		glog.Infof("Initial number of nodes: %d. New number of nodes: %d", clusterInitialTotalNodes, len(nodeList.Items))
		return len(nodeList.Items) > clusterInitialTotalNodes, nil
	})

	glog.Info("Delete workload")
	err = wait.PollImmediate(1*time.Second, waitMedium, func() (bool, error) {
		if err := tc.client.Delete(context.TODO(), &workLoad); err != nil {
			glog.Errorf("error querying api for workLoad object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	// We delete the clusterAutoscaler and ensure the initial number of replicas to get the cluster to the initial number of nodes
	// TODO: validate the autoscaler to scale down
	glog.Info("Delete clusterAutoscaler object")
	err = wait.PollImmediate(1*time.Second, waitShort, func() (bool, error) {
		if err := tc.client.Delete(context.TODO(), &clusterAutoscaler); err != nil {
			glog.Errorf("error querying api for clusterAutoscaler object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	glog.Info("Delete machineAutoscaler object")
	err = wait.PollImmediate(1*time.Second, waitShort, func() (bool, error) {
		if err := tc.client.Delete(context.TODO(), &machineAutoscaler); err != nil {
			glog.Errorf("error querying api for machineAutoscaler object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	glog.Infof("Ensure initial number of replicas: %d", initialNumberOfReplicas)
	err = wait.PollImmediate(1*time.Second, waitShort, func() (bool, error) {
		msKey := types.NamespacedName{
			Namespace: namespace,
			Name:      targetMachineSet.Name,
		}
		ms := &mapiv1beta1.MachineSet{}
		if err := tc.client.Get(context.TODO(), msKey, ms); err != nil {
			glog.Errorf("error querying api for machineSet object: %v, retrying...", err)
			return false, nil
		}
		ms.Spec.Replicas = initialNumberOfReplicas
		if err := tc.client.Update(context.TODO(), ms); err != nil {
			glog.Errorf("error querying api for machineSet object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	glog.Info("Wait for cluster to match initial number of nodes")
	return wait.PollImmediate(1*time.Second, waitLong, func() (bool, error) {
		nodeList := corev1.NodeList{}
		if err := tc.client.List(context.TODO(), &listOptions, &nodeList); err != nil {
			glog.Errorf("error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		glog.Infof("Initial number of nodes: %d. Current number of nodes: %d", clusterInitialTotalNodes, len(nodeList.Items))
		return len(nodeList.Items) == clusterInitialTotalNodes, nil
	})
}
