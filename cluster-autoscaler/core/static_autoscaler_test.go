/*
Copyright 2017 The Kubernetes Authors.

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

package core

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	mockprovider "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/mocks"
	testprovider "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/test"
	"k8s.io/autoscaler/cluster-autoscaler/clusterstate"
	clusterstate_utils "k8s.io/autoscaler/cluster-autoscaler/clusterstate/utils"
	"k8s.io/autoscaler/cluster-autoscaler/config"
	"k8s.io/autoscaler/cluster-autoscaler/context"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown/actuation"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown/deletiontracker"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown/legacy"
	. "k8s.io/autoscaler/cluster-autoscaler/core/test"
	core_utils "k8s.io/autoscaler/cluster-autoscaler/core/utils"
	"k8s.io/autoscaler/cluster-autoscaler/estimator"
	ca_processors "k8s.io/autoscaler/cluster-autoscaler/processors"
	"k8s.io/autoscaler/cluster-autoscaler/utils/kubernetes"
	kube_util "k8s.io/autoscaler/cluster-autoscaler/utils/kubernetes"
	. "k8s.io/autoscaler/cluster-autoscaler/utils/test"
	kube_record "k8s.io/client-go/tools/record"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
	v1appslister "k8s.io/client-go/listers/apps/v1"
	schedulerframework "k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	klog "k8s.io/klog/v2"
)

type podListerMock struct {
	mock.Mock
}

func (l *podListerMock) List() ([]*apiv1.Pod, error) {
	args := l.Called()
	return args.Get(0).([]*apiv1.Pod), args.Error(1)
}

type podDisruptionBudgetListerMock struct {
	mock.Mock
}

func (l *podDisruptionBudgetListerMock) List() ([]*policyv1.PodDisruptionBudget, error) {
	args := l.Called()
	return args.Get(0).([]*policyv1.PodDisruptionBudget), args.Error(1)
}

type daemonSetListerMock struct {
	mock.Mock
}

func (l *daemonSetListerMock) List(selector labels.Selector) ([]*appsv1.DaemonSet, error) {
	args := l.Called(selector)
	return args.Get(0).([]*appsv1.DaemonSet), args.Error(1)
}

func (l *daemonSetListerMock) DaemonSets(namespace string) v1appslister.DaemonSetNamespaceLister {
	args := l.Called(namespace)
	return args.Get(0).(v1appslister.DaemonSetNamespaceLister)
}

func (l *daemonSetListerMock) GetPodDaemonSets(pod *apiv1.Pod) ([]*appsv1.DaemonSet, error) {
	args := l.Called()
	return args.Get(0).([]*appsv1.DaemonSet), args.Error(1)
}

func (l *daemonSetListerMock) GetHistoryDaemonSets(history *appsv1.ControllerRevision) ([]*appsv1.DaemonSet, error) {
	args := l.Called()
	return args.Get(0).([]*appsv1.DaemonSet), args.Error(1)
}

type onScaleUpMock struct {
	mock.Mock
}

func (m *onScaleUpMock) ScaleUp(id string, delta int) error {
	klog.Infof("Scale up: %v %v", id, delta)
	args := m.Called(id, delta)
	return args.Error(0)
}

type onScaleDownMock struct {
	mock.Mock
}

func (m *onScaleDownMock) ScaleDown(id string, name string) error {
	klog.Infof("Scale down: %v %v", id, name)
	args := m.Called(id, name)
	return args.Error(0)
}

type onNodeGroupCreateMock struct {
	mock.Mock
}

func (m *onNodeGroupCreateMock) Create(id string) error {
	klog.Infof("Create group: %v", id)
	args := m.Called(id)
	return args.Error(0)
}

type onNodeGroupDeleteMock struct {
	mock.Mock
}

func (m *onNodeGroupDeleteMock) Delete(id string) error {
	klog.Infof("Delete group: %v", id)
	args := m.Called(id)
	return args.Error(0)
}

func TestStaticAutoscalerRunOnce(t *testing.T) {
	readyNodeLister := kubernetes.NewTestNodeLister(nil)
	allNodeLister := kubernetes.NewTestNodeLister(nil)
	scheduledPodMock := &podListerMock{}
	unschedulablePodMock := &podListerMock{}
	podDisruptionBudgetListerMock := &podDisruptionBudgetListerMock{}
	daemonSetListerMock := &daemonSetListerMock{}
	onScaleUpMock := &onScaleUpMock{}
	onScaleDownMock := &onScaleDownMock{}
	deleteFinished := make(chan bool, 1)

	n1 := BuildTestNode("n1", 1000, 1000)
	SetNodeReadyState(n1, true, time.Now())
	n2 := BuildTestNode("n2", 1000, 1000)
	SetNodeReadyState(n2, true, time.Now())
	n3 := BuildTestNode("n3", 1000, 1000)

	p1 := BuildTestPod("p1", 600, 100)
	p1.Spec.NodeName = "n1"
	p2 := BuildTestPod("p2", 600, 100)

	tn := BuildTestNode("tn", 1000, 1000)
	tni := schedulerframework.NewNodeInfo()
	tni.SetNode(tn)

	provider := testprovider.NewTestAutoprovisioningCloudProvider(
		func(id string, delta int) error {
			return onScaleUpMock.ScaleUp(id, delta)
		}, func(id string, name string) error {
			ret := onScaleDownMock.ScaleDown(id, name)
			deleteFinished <- true
			return ret
		},
		nil, nil,
		nil, map[string]*schedulerframework.NodeInfo{"ng1": tni, "ng2": tni})
	provider.AddNodeGroup("ng1", 1, 10, 1)
	provider.AddNode("ng1", n1)
	ng1 := reflect.ValueOf(provider.GetNodeGroup("ng1")).Interface().(*testprovider.TestNodeGroup)
	assert.NotNil(t, ng1)
	assert.NotNil(t, provider)

	// Create context with mocked lister registry.
	options := config.AutoscalingOptions{
		NodeGroupDefaults: config.NodeGroupAutoscalingOptions{
			ScaleDownUnneededTime:         time.Minute,
			ScaleDownUnreadyTime:          time.Minute,
			ScaleDownUtilizationThreshold: 0.5,
		},
		EstimatorName:    estimator.BinpackingEstimatorName,
		ScaleDownEnabled: true,
		MaxNodesTotal:    1,
		MaxCoresTotal:    10,
		MaxMemoryTotal:   100000,
	}
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()

	context, err := NewScaleTestAutoscalingContext(options, &fake.Clientset{}, nil, provider, processorCallbacks, nil)
	assert.NoError(t, err)

	listerRegistry := kube_util.NewListerRegistry(allNodeLister, readyNodeLister, scheduledPodMock,
		unschedulablePodMock, podDisruptionBudgetListerMock, daemonSetListerMock,
		nil, nil, nil, nil)
	context.ListerRegistry = listerRegistry

	clusterStateConfig := clusterstate.ClusterStateRegistryConfig{
		OkTotalUnreadyCount:  1,
		MaxNodeProvisionTime: 10 * time.Second,
	}

	processors := NewTestProcessors()
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())
	sdPlanner, sdActuator := newScaleDownPlannerAndActuator(t, &context, processors, clusterState)

	autoscaler := &StaticAutoscaler{
		AutoscalingContext:    &context,
		clusterStateRegistry:  clusterState,
		lastScaleUpTime:       time.Now(),
		lastScaleDownFailTime: time.Now(),
		scaleDownPlanner:      sdPlanner,
		scaleDownActuator:     sdActuator,
		processors:            processors,
		processorCallbacks:    processorCallbacks,
		initialized:           true,
	}

	// MaxNodesTotal reached.
	readyNodeLister.SetNodes([]*apiv1.Node{n1})
	allNodeLister.SetNodes([]*apiv1.Node{n1})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p2}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()

	err = autoscaler.RunOnce(time.Now())
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Scale up.
	readyNodeLister.SetNodes([]*apiv1.Node{n1})
	allNodeLister.SetNodes([]*apiv1.Node{n1})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Times(2) // 1 to get pods + 1 per nodegroup when building nodeInfo map
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p2}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	onScaleUpMock.On("ScaleUp", "ng1", 1).Return(nil).Once()

	context.MaxNodesTotal = 10
	err = autoscaler.RunOnce(time.Now().Add(time.Hour))
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Mark unneeded nodes.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()

	provider.AddNode("ng1", n2)
	ng1.SetTargetSize(2)

	err = autoscaler.RunOnce(time.Now().Add(2 * time.Hour))
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Scale down.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()
	onScaleDownMock.On("ScaleDown", "ng1", "n2").Return(nil).Once()

	err = autoscaler.RunOnce(time.Now().Add(3 * time.Hour))
	waitForDeleteToFinish(t, deleteFinished)
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Mark unregistered nodes.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p2}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()

	provider.AddNodeGroup("ng2", 0, 10, 1)
	provider.AddNode("ng2", n3)

	err = autoscaler.RunOnce(time.Now().Add(4 * time.Hour))
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Remove unregistered nodes.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p2}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	onScaleDownMock.On("ScaleDown", "ng2", "n3").Return(nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()

	err = autoscaler.RunOnce(time.Now().Add(5 * time.Hour))
	waitForDeleteToFinish(t, deleteFinished)
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)
}

func TestStaticAutoscalerRunOnceWithAutoprovisionedEnabled(t *testing.T) {
	readyNodeLister := kubernetes.NewTestNodeLister(nil)
	allNodeLister := kubernetes.NewTestNodeLister(nil)
	scheduledPodMock := &podListerMock{}
	unschedulablePodMock := &podListerMock{}
	podDisruptionBudgetListerMock := &podDisruptionBudgetListerMock{}
	daemonSetListerMock := &daemonSetListerMock{}
	onScaleUpMock := &onScaleUpMock{}
	onScaleDownMock := &onScaleDownMock{}
	onNodeGroupCreateMock := &onNodeGroupCreateMock{}
	onNodeGroupDeleteMock := &onNodeGroupDeleteMock{}
	nodeGroupManager := &MockAutoprovisioningNodeGroupManager{t, 0}
	nodeGroupListProcessor := &MockAutoprovisioningNodeGroupListProcessor{t}
	deleteFinished := make(chan bool, 1)

	n1 := BuildTestNode("n1", 100, 1000)
	SetNodeReadyState(n1, true, time.Now())
	n2 := BuildTestNode("n2", 1000, 1000)
	SetNodeReadyState(n2, true, time.Now())

	p1 := BuildTestPod("p1", 100, 100)
	p1.Spec.NodeName = "n1"
	p2 := BuildTestPod("p2", 600, 100)

	tn1 := BuildTestNode("tn1", 100, 1000)
	SetNodeReadyState(tn1, true, time.Now())
	tni1 := schedulerframework.NewNodeInfo()
	tni1.SetNode(tn1)
	tn2 := BuildTestNode("tn2", 1000, 1000)
	SetNodeReadyState(tn2, true, time.Now())
	tni2 := schedulerframework.NewNodeInfo()
	tni2.SetNode(tn2)
	tn3 := BuildTestNode("tn3", 100, 1000)
	SetNodeReadyState(tn2, true, time.Now())
	tni3 := schedulerframework.NewNodeInfo()
	tni3.SetNode(tn3)

	provider := testprovider.NewTestAutoprovisioningCloudProvider(
		func(id string, delta int) error {
			return onScaleUpMock.ScaleUp(id, delta)
		}, func(id string, name string) error {
			ret := onScaleDownMock.ScaleDown(id, name)
			deleteFinished <- true
			return ret
		}, func(id string) error {
			return onNodeGroupCreateMock.Create(id)
		}, func(id string) error {
			return onNodeGroupDeleteMock.Delete(id)
		},
		[]string{"TN1", "TN2"}, map[string]*schedulerframework.NodeInfo{"TN1": tni1, "TN2": tni2, "ng1": tni3})
	provider.AddNodeGroup("ng1", 1, 10, 1)
	provider.AddAutoprovisionedNodeGroup("autoprovisioned-TN1", 0, 10, 0, "TN1")
	autoprovisionedTN1 := reflect.ValueOf(provider.GetNodeGroup("autoprovisioned-TN1")).Interface().(*testprovider.TestNodeGroup)
	assert.NotNil(t, autoprovisionedTN1)
	provider.AddNode("ng1,", n1)
	assert.NotNil(t, provider)

	processors := NewTestProcessors()
	processors.NodeGroupManager = nodeGroupManager
	processors.NodeGroupListProcessor = nodeGroupListProcessor

	// Create context with mocked lister registry.
	options := config.AutoscalingOptions{
		NodeGroupDefaults: config.NodeGroupAutoscalingOptions{
			ScaleDownUnneededTime:         time.Minute,
			ScaleDownUnreadyTime:          time.Minute,
			ScaleDownUtilizationThreshold: 0.5,
		},
		EstimatorName:                    estimator.BinpackingEstimatorName,
		ScaleDownEnabled:                 true,
		MaxNodesTotal:                    100,
		MaxCoresTotal:                    100,
		MaxMemoryTotal:                   100000,
		NodeAutoprovisioningEnabled:      true,
		MaxAutoprovisionedNodeGroupCount: 10,
	}
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()

	context, err := NewScaleTestAutoscalingContext(options, &fake.Clientset{}, nil, provider, processorCallbacks, nil)
	assert.NoError(t, err)

	listerRegistry := kube_util.NewListerRegistry(allNodeLister, readyNodeLister, scheduledPodMock,
		unschedulablePodMock, podDisruptionBudgetListerMock, daemonSetListerMock,
		nil, nil, nil, nil)
	context.ListerRegistry = listerRegistry

	clusterStateConfig := clusterstate.ClusterStateRegistryConfig{
		OkTotalUnreadyCount:  0,
		MaxNodeProvisionTime: 10 * time.Second,
	}
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())

	sdPlanner, sdActuator := newScaleDownPlannerAndActuator(t, &context, processors, clusterState)

	autoscaler := &StaticAutoscaler{
		AutoscalingContext:    &context,
		clusterStateRegistry:  clusterState,
		lastScaleUpTime:       time.Now(),
		lastScaleDownFailTime: time.Now(),
		scaleDownPlanner:      sdPlanner,
		scaleDownActuator:     sdActuator,
		processors:            processors,
		processorCallbacks:    processorCallbacks,
		initialized:           true,
	}

	// Scale up.
	readyNodeLister.SetNodes([]*apiv1.Node{n1})
	allNodeLister.SetNodes([]*apiv1.Node{n1})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p2}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	onNodeGroupCreateMock.On("Create", "autoprovisioned-TN2").Return(nil).Once()
	onScaleUpMock.On("ScaleUp", "autoprovisioned-TN2", 1).Return(nil).Once()

	err = autoscaler.RunOnce(time.Now().Add(time.Hour))
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Fix target size.
	autoprovisionedTN1.SetTargetSize(0)

	// Remove autoprovisioned node group and mark unneeded nodes.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	onNodeGroupDeleteMock.On("Delete", "autoprovisioned-TN1").Return(nil).Once()

	provider.AddAutoprovisionedNodeGroup("autoprovisioned-TN2", 0, 10, 1, "TN1")
	provider.AddNode("autoprovisioned-TN2", n2)

	err = autoscaler.RunOnce(time.Now().Add(1 * time.Hour))
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Scale down.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	onNodeGroupDeleteMock.On("Delete", "autoprovisioned-"+
		"TN1").Return(nil).Once()
	onScaleDownMock.On("ScaleDown", "autoprovisioned-TN2", "n2").Return(nil).Once()

	err = autoscaler.RunOnce(time.Now().Add(2 * time.Hour))
	waitForDeleteToFinish(t, deleteFinished)
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)
}

func TestStaticAutoscalerRunOnceWithALongUnregisteredNode(t *testing.T) {
	readyNodeLister := kubernetes.NewTestNodeLister(nil)
	allNodeLister := kubernetes.NewTestNodeLister(nil)
	scheduledPodMock := &podListerMock{}
	unschedulablePodMock := &podListerMock{}
	podDisruptionBudgetListerMock := &podDisruptionBudgetListerMock{}
	daemonSetListerMock := &daemonSetListerMock{}
	onScaleUpMock := &onScaleUpMock{}
	onScaleDownMock := &onScaleDownMock{}
	deleteFinished := make(chan bool, 1)

	now := time.Now()
	later := now.Add(1 * time.Minute)

	n1 := BuildTestNode("n1", 1000, 1000)
	SetNodeReadyState(n1, true, time.Now())
	n2 := BuildTestNode("n2", 1000, 1000)
	SetNodeReadyState(n2, true, time.Now())

	p1 := BuildTestPod("p1", 600, 100)
	p1.Spec.NodeName = "n1"
	p2 := BuildTestPod("p2", 600, 100)

	provider := testprovider.NewTestCloudProvider(
		func(id string, delta int) error {
			return onScaleUpMock.ScaleUp(id, delta)
		}, func(id string, name string) error {
			ret := onScaleDownMock.ScaleDown(id, name)
			deleteFinished <- true
			return ret
		})
	provider.AddNodeGroup("ng1", 2, 10, 2)
	provider.AddNode("ng1", n1)

	// broken node, that will be just hanging out there during
	// the test (it can't be removed since that would validate group min size)
	brokenNode := BuildTestNode("broken", 1000, 1000)
	provider.AddNode("ng1", brokenNode)

	ng1 := reflect.ValueOf(provider.GetNodeGroup("ng1")).Interface().(*testprovider.TestNodeGroup)
	assert.NotNil(t, ng1)
	assert.NotNil(t, provider)

	// Create context with mocked lister registry.
	options := config.AutoscalingOptions{
		NodeGroupDefaults: config.NodeGroupAutoscalingOptions{
			ScaleDownUnneededTime:         time.Minute,
			ScaleDownUnreadyTime:          time.Minute,
			ScaleDownUtilizationThreshold: 0.5,
		},
		EstimatorName:        estimator.BinpackingEstimatorName,
		ScaleDownEnabled:     true,
		MaxNodesTotal:        10,
		MaxCoresTotal:        10,
		MaxMemoryTotal:       100000,
		MaxNodeProvisionTime: 10 * time.Second,
	}
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()

	context, err := NewScaleTestAutoscalingContext(options, &fake.Clientset{}, nil, provider, processorCallbacks, nil)
	assert.NoError(t, err)

	listerRegistry := kube_util.NewListerRegistry(allNodeLister, readyNodeLister, scheduledPodMock,
		unschedulablePodMock, podDisruptionBudgetListerMock, daemonSetListerMock,
		nil, nil, nil, nil)
	context.ListerRegistry = listerRegistry

	clusterStateConfig := clusterstate.ClusterStateRegistryConfig{
		OkTotalUnreadyCount:  1,
		MaxNodeProvisionTime: 10 * time.Second,
	}
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())
	// broken node detected as unregistered

	nodes := []*apiv1.Node{n1}
	// nodeInfos, _ := getNodeInfosForGroups(nodes, provider, listerRegistry, []*appsv1.DaemonSet{}, context.PredicateChecker)
	clusterState.UpdateNodes(nodes, nil, now)

	// broken node failed to register in time
	clusterState.UpdateNodes(nodes, nil, later)

	processors := NewTestProcessors()

	sdPlanner, sdActuator := newScaleDownPlannerAndActuator(t, &context, processors, clusterState)

	autoscaler := &StaticAutoscaler{
		AutoscalingContext:    &context,
		clusterStateRegistry:  clusterState,
		lastScaleUpTime:       time.Now(),
		lastScaleDownFailTime: time.Now(),
		scaleDownPlanner:      sdPlanner,
		scaleDownActuator:     sdActuator,
		processors:            processors,
		processorCallbacks:    processorCallbacks,
	}

	// Scale up.
	readyNodeLister.SetNodes([]*apiv1.Node{n1})
	allNodeLister.SetNodes([]*apiv1.Node{n1})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice() // 1 to get pods + 1 per nodegroup when building nodeInfo map
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p2}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	onScaleUpMock.On("ScaleUp", "ng1", 1).Return(nil).Once()

	err = autoscaler.RunOnce(later.Add(time.Hour))
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Remove broken node after going over min size
	provider.AddNode("ng1", n2)
	ng1.SetTargetSize(3)

	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Twice()
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p2}, nil).Once()
	onScaleDownMock.On("ScaleDown", "ng1", "broken").Return(nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()

	err = autoscaler.RunOnce(later.Add(2 * time.Hour))
	waitForDeleteToFinish(t, deleteFinished)
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)
}

func TestStaticAutoscalerRunOncePodsWithPriorities(t *testing.T) {
	readyNodeLister := kubernetes.NewTestNodeLister(nil)
	allNodeLister := kubernetes.NewTestNodeLister(nil)
	scheduledPodMock := &podListerMock{}
	unschedulablePodMock := &podListerMock{}
	podDisruptionBudgetListerMock := &podDisruptionBudgetListerMock{}
	daemonSetListerMock := &daemonSetListerMock{}
	onScaleUpMock := &onScaleUpMock{}
	onScaleDownMock := &onScaleDownMock{}
	deleteFinished := make(chan bool, 1)

	n1 := BuildTestNode("n1", 100, 1000)
	SetNodeReadyState(n1, true, time.Now())
	n2 := BuildTestNode("n2", 1000, 1000)
	SetNodeReadyState(n2, true, time.Now())
	n3 := BuildTestNode("n3", 1000, 1000)
	SetNodeReadyState(n3, true, time.Now())

	// shared owner reference
	ownerRef := GenerateOwnerReferences("rs", "ReplicaSet", "extensions/v1beta1", "")
	var priority100 int32 = 100
	var priority1 int32 = 1

	p1 := BuildTestPod("p1", 40, 0)
	p1.OwnerReferences = ownerRef
	p1.Spec.NodeName = "n1"
	p1.Spec.Priority = &priority1

	p2 := BuildTestPod("p2", 400, 0)
	p2.OwnerReferences = ownerRef
	p2.Spec.NodeName = "n2"
	p2.Spec.Priority = &priority1

	p3 := BuildTestPod("p3", 400, 0)
	p3.OwnerReferences = ownerRef
	p3.Spec.NodeName = "n2"
	p3.Spec.Priority = &priority100

	p4 := BuildTestPod("p4", 500, 0)
	p4.OwnerReferences = ownerRef
	p4.Spec.Priority = &priority100

	p5 := BuildTestPod("p5", 800, 0)
	p5.OwnerReferences = ownerRef
	p5.Spec.Priority = &priority100
	p5.Status.NominatedNodeName = "n3"

	p6 := BuildTestPod("p6", 1000, 0)
	p6.OwnerReferences = ownerRef
	p6.Spec.Priority = &priority100

	provider := testprovider.NewTestCloudProvider(
		func(id string, delta int) error {
			return onScaleUpMock.ScaleUp(id, delta)
		}, func(id string, name string) error {
			ret := onScaleDownMock.ScaleDown(id, name)
			deleteFinished <- true
			return ret
		})
	provider.AddNodeGroup("ng1", 0, 10, 1)
	provider.AddNodeGroup("ng2", 0, 10, 2)
	provider.AddNode("ng1", n1)
	provider.AddNode("ng2", n2)
	provider.AddNode("ng2", n3)
	assert.NotNil(t, provider)
	ng2 := reflect.ValueOf(provider.GetNodeGroup("ng2")).Interface().(*testprovider.TestNodeGroup)
	assert.NotNil(t, ng2)

	// Create context with mocked lister registry.
	options := config.AutoscalingOptions{
		NodeGroupDefaults: config.NodeGroupAutoscalingOptions{
			ScaleDownUnneededTime:         time.Minute,
			ScaleDownUtilizationThreshold: 0.5,
			ScaleDownUnreadyTime:          time.Minute,
		},
		EstimatorName:                estimator.BinpackingEstimatorName,
		ScaleDownEnabled:             true,
		MaxNodesTotal:                10,
		MaxCoresTotal:                10,
		MaxMemoryTotal:               100000,
		ExpendablePodsPriorityCutoff: 10,
	}
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()

	context, err := NewScaleTestAutoscalingContext(options, &fake.Clientset{}, nil, provider, processorCallbacks, nil)
	assert.NoError(t, err)

	listerRegistry := kube_util.NewListerRegistry(allNodeLister, readyNodeLister, scheduledPodMock,
		unschedulablePodMock, podDisruptionBudgetListerMock, daemonSetListerMock,
		nil, nil, nil, nil)
	context.ListerRegistry = listerRegistry

	clusterStateConfig := clusterstate.ClusterStateRegistryConfig{
		OkTotalUnreadyCount:  1,
		MaxNodeProvisionTime: 10 * time.Second,
	}

	processors := NewTestProcessors()
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())
	sdPlanner, sdActuator := newScaleDownPlannerAndActuator(t, &context, processors, clusterState)

	autoscaler := &StaticAutoscaler{
		AutoscalingContext:    &context,
		clusterStateRegistry:  clusterState,
		lastScaleUpTime:       time.Now(),
		lastScaleDownFailTime: time.Now(),
		scaleDownPlanner:      sdPlanner,
		scaleDownActuator:     sdActuator,
		processors:            processors,
		processorCallbacks:    processorCallbacks,
	}

	// Scale up
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2, n3})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2, n3})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1, p2, p3}, nil).Times(2) // 1 to get pods + 1 when building nodeInfo map
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p4, p5, p6}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	onScaleUpMock.On("ScaleUp", "ng2", 1).Return(nil).Once()

	err = autoscaler.RunOnce(time.Now())
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Mark unneeded nodes.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2, n3})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2, n3})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1, p2, p3}, nil).Times(2)
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p4, p5}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()

	ng2.SetTargetSize(2)

	err = autoscaler.RunOnce(time.Now().Add(2 * time.Hour))
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)

	// Scale down.
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2, n3})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2, n3})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p1, p2, p3, p4}, nil).Times(2)
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p5}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()
	podDisruptionBudgetListerMock.On("List").Return([]*policyv1.PodDisruptionBudget{}, nil).Once()
	onScaleDownMock.On("ScaleDown", "ng1", "n1").Return(nil).Once()

	p4.Spec.NodeName = "n2"

	err = autoscaler.RunOnce(time.Now().Add(3 * time.Hour))
	waitForDeleteToFinish(t, deleteFinished)
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)
}

func TestStaticAutoscalerRunOnceWithFilteringOnBinPackingEstimator(t *testing.T) {
	readyNodeLister := kubernetes.NewTestNodeLister(nil)
	allNodeLister := kubernetes.NewTestNodeLister(nil)
	scheduledPodMock := &podListerMock{}
	unschedulablePodMock := &podListerMock{}
	podDisruptionBudgetListerMock := &podDisruptionBudgetListerMock{}
	daemonSetListerMock := &daemonSetListerMock{}
	onScaleUpMock := &onScaleUpMock{}
	onScaleDownMock := &onScaleDownMock{}

	n1 := BuildTestNode("n1", 2000, 1000)
	SetNodeReadyState(n1, true, time.Now())
	n2 := BuildTestNode("n2", 2000, 1000)
	SetNodeReadyState(n2, true, time.Now())

	// shared owner reference
	ownerRef := GenerateOwnerReferences("rs", "ReplicaSet", "extensions/v1beta1", "")

	p1 := BuildTestPod("p1", 1400, 0)
	p1.OwnerReferences = ownerRef
	p3 := BuildTestPod("p3", 1400, 0)
	p3.Spec.NodeName = "n1"
	p3.OwnerReferences = ownerRef
	p4 := BuildTestPod("p4", 1400, 0)
	p4.Spec.NodeName = "n2"
	p4.OwnerReferences = ownerRef

	provider := testprovider.NewTestCloudProvider(
		func(id string, delta int) error {
			return onScaleUpMock.ScaleUp(id, delta)
		}, func(id string, name string) error {
			return onScaleDownMock.ScaleDown(id, name)
		})
	provider.AddNodeGroup("ng1", 0, 10, 2)
	provider.AddNode("ng1", n1)

	provider.AddNodeGroup("ng2", 0, 10, 1)
	provider.AddNode("ng2", n2)

	assert.NotNil(t, provider)

	// Create context with mocked lister registry.
	options := config.AutoscalingOptions{
		NodeGroupDefaults: config.NodeGroupAutoscalingOptions{
			ScaleDownUtilizationThreshold: 0.5,
		},
		EstimatorName:                estimator.BinpackingEstimatorName,
		ScaleDownEnabled:             false,
		MaxNodesTotal:                10,
		MaxCoresTotal:                10,
		MaxMemoryTotal:               100000,
		ExpendablePodsPriorityCutoff: 10,
	}
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()

	context, err := NewScaleTestAutoscalingContext(options, &fake.Clientset{}, nil, provider, processorCallbacks, nil)
	assert.NoError(t, err)

	listerRegistry := kube_util.NewListerRegistry(allNodeLister, readyNodeLister, scheduledPodMock,
		unschedulablePodMock, podDisruptionBudgetListerMock, daemonSetListerMock,
		nil, nil, nil, nil)
	context.ListerRegistry = listerRegistry

	clusterStateConfig := clusterstate.ClusterStateRegistryConfig{
		OkTotalUnreadyCount:  1,
		MaxNodeProvisionTime: 10 * time.Second,
	}

	processors := NewTestProcessors()
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())
	sdPlanner, sdActuator := newScaleDownPlannerAndActuator(t, &context, processors, clusterState)

	autoscaler := &StaticAutoscaler{
		AutoscalingContext:    &context,
		clusterStateRegistry:  clusterState,
		lastScaleUpTime:       time.Now(),
		lastScaleDownFailTime: time.Now(),
		scaleDownPlanner:      sdPlanner,
		scaleDownActuator:     sdActuator,
		processors:            processors,
		processorCallbacks:    processorCallbacks,
	}

	// Scale up
	readyNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	allNodeLister.SetNodes([]*apiv1.Node{n1, n2})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p3, p4}, nil).Times(2) // 1 to get pods + 1 when building nodeInfo map
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()

	err = autoscaler.RunOnce(time.Now())
	assert.NoError(t, err)

	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)
}

func TestStaticAutoscalerRunOnceWithFilteringOnUpcomingNodesEnabledNoScaleUp(t *testing.T) {
	readyNodeLister := kubernetes.NewTestNodeLister(nil)
	allNodeLister := kubernetes.NewTestNodeLister(nil)
	scheduledPodMock := &podListerMock{}
	unschedulablePodMock := &podListerMock{}
	podDisruptionBudgetListerMock := &podDisruptionBudgetListerMock{}
	daemonSetListerMock := &daemonSetListerMock{}
	onScaleUpMock := &onScaleUpMock{}
	onScaleDownMock := &onScaleDownMock{}

	n2 := BuildTestNode("n2", 2000, 1000)
	SetNodeReadyState(n2, true, time.Now())
	n3 := BuildTestNode("n3", 2000, 1000)
	SetNodeReadyState(n3, true, time.Now())

	// shared owner reference
	ownerRef := GenerateOwnerReferences("rs", "ReplicaSet", "extensions/v1beta1", "")

	p1 := BuildTestPod("p1", 1400, 0)
	p1.OwnerReferences = ownerRef
	p2 := BuildTestPod("p2", 1400, 0)
	p2.Spec.NodeName = "n2"
	p2.OwnerReferences = ownerRef
	p3 := BuildTestPod("p3", 1400, 0)
	p3.Spec.NodeName = "n3"
	p3.OwnerReferences = ownerRef

	provider := testprovider.NewTestCloudProvider(
		func(id string, delta int) error {
			return onScaleUpMock.ScaleUp(id, delta)
		}, func(id string, name string) error {
			return onScaleDownMock.ScaleDown(id, name)
		})
	provider.AddNodeGroup("ng1", 0, 10, 2)
	provider.AddNode("ng1", n2)

	provider.AddNodeGroup("ng2", 0, 10, 1)
	provider.AddNode("ng2", n3)

	assert.NotNil(t, provider)

	// Create context with mocked lister registry.
	options := config.AutoscalingOptions{
		NodeGroupDefaults: config.NodeGroupAutoscalingOptions{
			ScaleDownUtilizationThreshold: 0.5,
		},
		EstimatorName:                estimator.BinpackingEstimatorName,
		ScaleDownEnabled:             false,
		MaxNodesTotal:                10,
		MaxCoresTotal:                10,
		MaxMemoryTotal:               100000,
		ExpendablePodsPriorityCutoff: 10,
	}
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()

	context, err := NewScaleTestAutoscalingContext(options, &fake.Clientset{}, nil, provider, processorCallbacks, nil)
	assert.NoError(t, err)

	listerRegistry := kube_util.NewListerRegistry(allNodeLister, readyNodeLister, scheduledPodMock,
		unschedulablePodMock, podDisruptionBudgetListerMock, daemonSetListerMock,
		nil, nil, nil, nil)
	context.ListerRegistry = listerRegistry

	clusterStateConfig := clusterstate.ClusterStateRegistryConfig{
		OkTotalUnreadyCount:  1,
		MaxNodeProvisionTime: 10 * time.Second,
	}

	processors := NewTestProcessors()
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())
	sdPlanner, sdActuator := newScaleDownPlannerAndActuator(t, &context, processors, clusterState)

	autoscaler := &StaticAutoscaler{
		AutoscalingContext:    &context,
		clusterStateRegistry:  clusterState,
		lastScaleUpTime:       time.Now(),
		lastScaleDownFailTime: time.Now(),
		scaleDownPlanner:      sdPlanner,
		scaleDownActuator:     sdActuator,
		processors:            processors,
		processorCallbacks:    processorCallbacks,
	}

	// Scale up
	readyNodeLister.SetNodes([]*apiv1.Node{n2, n3})
	allNodeLister.SetNodes([]*apiv1.Node{n2, n3})
	scheduledPodMock.On("List").Return([]*apiv1.Pod{p2, p3}, nil).Times(2) // 1 to get pods + 1 when building nodeInfo map
	unschedulablePodMock.On("List").Return([]*apiv1.Pod{p1}, nil).Once()
	daemonSetListerMock.On("List", labels.Everything()).Return([]*appsv1.DaemonSet{}, nil).Once()

	err = autoscaler.RunOnce(time.Now())
	assert.NoError(t, err)

	mock.AssertExpectationsForObjects(t, scheduledPodMock, unschedulablePodMock,
		podDisruptionBudgetListerMock, daemonSetListerMock, onScaleUpMock, onScaleDownMock)
}

func TestStaticAutoscalerInstanceCreationErrors(t *testing.T) {

	// setup
	provider := &mockprovider.CloudProvider{}

	// Create context with mocked lister registry.
	options := config.AutoscalingOptions{
		NodeGroupDefaults: config.NodeGroupAutoscalingOptions{
			ScaleDownUnneededTime:         time.Minute,
			ScaleDownUnreadyTime:          time.Minute,
			ScaleDownUtilizationThreshold: 0.5,
		},
		EstimatorName:                estimator.BinpackingEstimatorName,
		ScaleDownEnabled:             true,
		MaxNodesTotal:                10,
		MaxCoresTotal:                10,
		MaxMemoryTotal:               100000,
		ExpendablePodsPriorityCutoff: 10,
	}
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()

	context, err := NewScaleTestAutoscalingContext(options, &fake.Clientset{}, nil, provider, processorCallbacks, nil)
	assert.NoError(t, err)

	clusterStateConfig := clusterstate.ClusterStateRegistryConfig{
		OkTotalUnreadyCount:  1,
		MaxNodeProvisionTime: 10 * time.Second,
	}

	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())
	autoscaler := &StaticAutoscaler{
		AutoscalingContext:    &context,
		clusterStateRegistry:  clusterState,
		lastScaleUpTime:       time.Now(),
		lastScaleDownFailTime: time.Now(),
		processorCallbacks:    processorCallbacks,
	}

	nodeGroupA := &mockprovider.NodeGroup{}
	nodeGroupB := &mockprovider.NodeGroup{}

	// Three nodes with out-of-resources errors
	nodeGroupA.On("Exist").Return(true)
	nodeGroupA.On("Autoprovisioned").Return(false)
	nodeGroupA.On("TargetSize").Return(5, nil)
	nodeGroupA.On("Id").Return("A")
	nodeGroupA.On("DeleteNodes", mock.Anything).Return(nil)
	nodeGroupA.On("Nodes").Return([]cloudprovider.Instance{
		{
			Id: "A1",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceRunning,
			},
		},
		{
			Id: "A2",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceCreating,
			},
		},
		{
			Id: "A3",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceCreating,
				ErrorInfo: &cloudprovider.InstanceErrorInfo{
					ErrorClass: cloudprovider.OutOfResourcesErrorClass,
					ErrorCode:  "RESOURCE_POOL_EXHAUSTED",
				},
			},
		},
		{
			Id: "A4",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceCreating,
				ErrorInfo: &cloudprovider.InstanceErrorInfo{
					ErrorClass: cloudprovider.OutOfResourcesErrorClass,
					ErrorCode:  "RESOURCE_POOL_EXHAUSTED",
				},
			},
		},
		{
			Id: "A5",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceCreating,
				ErrorInfo: &cloudprovider.InstanceErrorInfo{
					ErrorClass: cloudprovider.OutOfResourcesErrorClass,
					ErrorCode:  "QUOTA",
				},
			},
		},
		{
			Id: "A6",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceCreating,
				ErrorInfo: &cloudprovider.InstanceErrorInfo{
					ErrorClass: cloudprovider.OtherErrorClass,
					ErrorCode:  "OTHER",
				},
			},
		},
	}, nil).Twice()

	nodeGroupB.On("Exist").Return(true)
	nodeGroupB.On("Autoprovisioned").Return(false)
	nodeGroupB.On("TargetSize").Return(5, nil)
	nodeGroupB.On("Id").Return("B")
	nodeGroupB.On("DeleteNodes", mock.Anything).Return(nil)
	nodeGroupB.On("Nodes").Return([]cloudprovider.Instance{
		{
			Id: "B1",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceRunning,
			},
		},
	}, nil)

	provider.On("NodeGroups").Return([]cloudprovider.NodeGroup{nodeGroupA})
	provider.On("NodeGroupForNode", mock.Anything).Return(
		func(node *apiv1.Node) cloudprovider.NodeGroup {
			if strings.HasPrefix(node.Spec.ProviderID, "A") {
				return nodeGroupA
			}
			if strings.HasPrefix(node.Spec.ProviderID, "B") {
				return nodeGroupB
			}
			return nil
		}, nil)

	now := time.Now()

	clusterState.RefreshCloudProviderNodeInstancesCache()
	// propagate nodes info in cluster state
	clusterState.UpdateNodes([]*apiv1.Node{}, nil, now)

	// delete nodes with create errors
	removedNodes, err := autoscaler.deleteCreatedNodesWithErrors()
	assert.True(t, removedNodes)
	assert.NoError(t, err)

	// check delete was called on correct nodes
	nodeGroupA.AssertCalled(t, "DeleteNodes", mock.MatchedBy(
		func(nodes []*apiv1.Node) bool {
			if len(nodes) != 4 {
				return false
			}
			names := make(map[string]bool)
			for _, node := range nodes {
				names[node.Spec.ProviderID] = true
			}
			return names["A3"] && names["A4"] && names["A5"] && names["A6"]
		}))

	// TODO assert that scaleup was failed (separately for QUOTA and RESOURCE_POOL_EXHAUSTED)

	clusterState.RefreshCloudProviderNodeInstancesCache()

	// propagate nodes info in cluster state again
	// no changes in what provider returns
	clusterState.UpdateNodes([]*apiv1.Node{}, nil, now)

	// delete nodes with create errors
	removedNodes, err = autoscaler.deleteCreatedNodesWithErrors()
	assert.True(t, removedNodes)
	assert.NoError(t, err)

	// nodes should be deleted again
	nodeGroupA.AssertCalled(t, "DeleteNodes", mock.MatchedBy(
		func(nodes []*apiv1.Node) bool {
			if len(nodes) != 4 {
				return false
			}
			names := make(map[string]bool)
			for _, node := range nodes {
				names[node.Spec.ProviderID] = true
			}
			return names["A3"] && names["A4"] && names["A5"] && names["A6"]
		}))

	// TODO assert that scaleup is not failed again

	// restub node group A so nodes are no longer reporting errors
	nodeGroupA.On("Nodes").Return([]cloudprovider.Instance{
		{
			Id: "A1",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceRunning,
			},
		},
		{
			Id: "A2",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceCreating,
			},
		},
		{
			Id: "A3",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceDeleting,
			},
		},
		{
			Id: "A4",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceDeleting,
			},
		},
		{
			Id: "A5",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceDeleting,
			},
		},
		{
			Id: "A6",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceDeleting,
			},
		},
	}, nil)

	clusterState.RefreshCloudProviderNodeInstancesCache()

	// update cluster state
	clusterState.UpdateNodes([]*apiv1.Node{}, nil, now)

	// delete nodes with create errors
	removedNodes, err = autoscaler.deleteCreatedNodesWithErrors()
	assert.False(t, removedNodes)
	assert.NoError(t, err)

	// we expect no more Delete Nodes
	nodeGroupA.AssertNumberOfCalls(t, "DeleteNodes", 2)

	// failed node not included by NodeGroupForNode
	nodeGroupC := &mockprovider.NodeGroup{}
	nodeGroupC.On("Exist").Return(true)
	nodeGroupC.On("Autoprovisioned").Return(false)
	nodeGroupC.On("TargetSize").Return(1, nil)
	nodeGroupC.On("Id").Return("C")
	nodeGroupC.On("DeleteNodes", mock.Anything).Return(nil)
	nodeGroupC.On("Nodes").Return([]cloudprovider.Instance{
		{
			Id: "C1",
			Status: &cloudprovider.InstanceStatus{
				State: cloudprovider.InstanceCreating,
				ErrorInfo: &cloudprovider.InstanceErrorInfo{
					ErrorClass: cloudprovider.OutOfResourcesErrorClass,
					ErrorCode:  "QUOTA",
				},
			},
		},
	}, nil)
	provider = &mockprovider.CloudProvider{}
	provider.On("NodeGroups").Return([]cloudprovider.NodeGroup{nodeGroupC})
	provider.On("NodeGroupForNode", mock.Anything).Return(nil, nil)

	clusterState = clusterstate.NewClusterStateRegistry(provider, clusterStateConfig, context.LogRecorder, NewBackoff())
	clusterState.RefreshCloudProviderNodeInstancesCache()
	autoscaler.clusterStateRegistry = clusterState

	// update cluster state
	clusterState.UpdateNodes([]*apiv1.Node{}, nil, time.Now())

	// return early on failed nodes without matching nodegroups
	removedNodes, err = autoscaler.deleteCreatedNodesWithErrors()
	assert.False(t, removedNodes)
	assert.Error(t, err)
	nodeGroupC.AssertNumberOfCalls(t, "DeleteNodes", 0)
}

func TestStaticAutoscalerProcessorCallbacks(t *testing.T) {
	processorCallbacks := newStaticAutoscalerProcessorCallbacks()
	assert.Equal(t, false, processorCallbacks.disableScaleDownForLoop)
	assert.Equal(t, 0, len(processorCallbacks.extraValues))

	processorCallbacks.DisableScaleDownForLoop()
	assert.Equal(t, true, processorCallbacks.disableScaleDownForLoop)
	processorCallbacks.reset()
	assert.Equal(t, false, processorCallbacks.disableScaleDownForLoop)

	_, found := processorCallbacks.GetExtraValue("blah")
	assert.False(t, found)

	processorCallbacks.SetExtraValue("blah", "some value")
	value, found := processorCallbacks.GetExtraValue("blah")
	assert.True(t, found)
	assert.Equal(t, "some value", value)

	processorCallbacks.reset()
	assert.Equal(t, 0, len(processorCallbacks.extraValues))
	_, found = processorCallbacks.GetExtraValue("blah")
	assert.False(t, found)
}

func TestRemoveFixNodeTargetSize(t *testing.T) {
	sizeChanges := make(chan string, 10)
	now := time.Now()

	ng1_1 := BuildTestNode("ng1-1", 1000, 1000)
	ng1_1.Spec.ProviderID = "ng1-1"
	provider := testprovider.NewTestCloudProvider(func(nodegroup string, delta int) error {
		sizeChanges <- fmt.Sprintf("%s/%d", nodegroup, delta)
		return nil
	}, nil)
	provider.AddNodeGroup("ng1", 1, 10, 3)
	provider.AddNode("ng1", ng1_1)

	fakeClient := &fake.Clientset{}
	fakeLogRecorder, _ := clusterstate_utils.NewStatusMapRecorder(fakeClient, "kube-system", kube_record.NewFakeRecorder(5), false, "my-cool-configmap")
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterstate.ClusterStateRegistryConfig{
		MaxTotalUnreadyPercentage: 10,
		OkTotalUnreadyCount:       1,
	}, fakeLogRecorder, NewBackoff())
	err := clusterState.UpdateNodes([]*apiv1.Node{ng1_1}, nil, now.Add(-time.Hour))
	assert.NoError(t, err)

	context := &context.AutoscalingContext{
		AutoscalingOptions: config.AutoscalingOptions{
			MaxNodeProvisionTime: 45 * time.Minute,
		},
		CloudProvider: provider,
	}

	// Nothing should be fixed. The incorrect size state is not old enough.
	removed, err := fixNodeGroupSize(context, clusterState, now.Add(-50*time.Minute))
	assert.NoError(t, err)
	assert.False(t, removed)

	// Node group should be decreased.
	removed, err = fixNodeGroupSize(context, clusterState, now)
	assert.NoError(t, err)
	assert.True(t, removed)
	change := core_utils.GetStringFromChan(sizeChanges)
	assert.Equal(t, "ng1/-2", change)
}

func TestRemoveOldUnregisteredNodes(t *testing.T) {
	deletedNodes := make(chan string, 10)

	now := time.Now()

	ng1_1 := BuildTestNode("ng1-1", 1000, 1000)
	ng1_1.Spec.ProviderID = "ng1-1"
	ng1_2 := BuildTestNode("ng1-2", 1000, 1000)
	ng1_2.Spec.ProviderID = "ng1-2"
	provider := testprovider.NewTestCloudProvider(nil, func(nodegroup string, node string) error {
		deletedNodes <- fmt.Sprintf("%s/%s", nodegroup, node)
		return nil
	})
	provider.AddNodeGroup("ng1", 1, 10, 2)
	provider.AddNode("ng1", ng1_1)
	provider.AddNode("ng1", ng1_2)

	fakeClient := &fake.Clientset{}
	fakeLogRecorder, _ := clusterstate_utils.NewStatusMapRecorder(fakeClient, "kube-system", kube_record.NewFakeRecorder(5), false, "my-cool-configmap")
	clusterState := clusterstate.NewClusterStateRegistry(provider, clusterstate.ClusterStateRegistryConfig{
		MaxTotalUnreadyPercentage: 10,
		OkTotalUnreadyCount:       1,
	}, fakeLogRecorder, NewBackoff())
	err := clusterState.UpdateNodes([]*apiv1.Node{ng1_1}, nil, now.Add(-time.Hour))
	assert.NoError(t, err)

	context := &context.AutoscalingContext{
		AutoscalingOptions: config.AutoscalingOptions{
			MaxNodeProvisionTime: 45 * time.Minute,
		},
		CloudProvider: provider,
	}
	unregisteredNodes := clusterState.GetUnregisteredNodes()
	assert.Equal(t, 1, len(unregisteredNodes))

	// Nothing should be removed. The unregistered node is not old enough.
	removed, err := removeOldUnregisteredNodes(unregisteredNodes, context, clusterState, now.Add(-50*time.Minute), fakeLogRecorder)
	assert.NoError(t, err)
	assert.False(t, removed)

	// ng1_2 should be removed.
	removed, err = removeOldUnregisteredNodes(unregisteredNodes, context, clusterState, now, fakeLogRecorder)
	assert.NoError(t, err)
	assert.True(t, removed)
	deletedNode := core_utils.GetStringFromChan(deletedNodes)
	assert.Equal(t, "ng1/ng1-2", deletedNode)
}

func TestSubtractNodes(t *testing.T) {
	ns := make([]*apiv1.Node, 5)
	for i := 0; i < len(ns); i++ {
		ns[i] = BuildTestNode(fmt.Sprintf("n%d", i), 1000, 1000)
	}
	testCases := []struct {
		a []*apiv1.Node
		b []*apiv1.Node
		c []*apiv1.Node
	}{
		{
			a: ns,
			b: nil,
			c: ns,
		},
		{
			a: nil,
			b: ns,
			c: nil,
		},
		{
			a: ns,
			b: []*apiv1.Node{ns[3]},
			c: []*apiv1.Node{ns[0], ns[1], ns[2], ns[4]},
		},
		{
			a: ns,
			b: []*apiv1.Node{ns[0], ns[1], ns[2], ns[4]},
			c: []*apiv1.Node{ns[3]},
		},
		{
			a: []*apiv1.Node{ns[3]},
			b: []*apiv1.Node{ns[0], ns[1], ns[2], ns[4]},
			c: []*apiv1.Node{ns[3]},
		},
	}
	for _, tc := range testCases {
		got := subtractNodes(tc.a, tc.b)
		assert.Equal(t, nodeNames(got), nodeNames(tc.c))
	}
}

func nodeNames(ns []*apiv1.Node) []string {
	names := make([]string, len(ns))
	for i, node := range ns {
		names[i] = node.Name
	}
	return names
}

func waitForDeleteToFinish(t *testing.T, deleteFinished <-chan bool) {
	select {
	case <-deleteFinished:
		return
	case <-time.After(20 * time.Second):
		t.Fatalf("Node delete not finished")
	}
}

func newScaleDownPlannerAndActuator(t *testing.T, ctx *context.AutoscalingContext, p *ca_processors.AutoscalingProcessors, cs *clusterstate.ClusterStateRegistry) (scaledown.Planner, scaledown.Actuator) {
	ctx.MaxScaleDownParallelism = 10
	ctx.MaxDrainParallelism = 1
	ndt := deletiontracker.NewNodeDeletionTracker(0 * time.Second)
	sd := legacy.NewScaleDown(ctx, p, cs, ndt)
	actuator := actuation.NewActuator(ctx, cs, ndt)
	wrapper := legacy.NewScaleDownWrapper(sd, actuator)
	return wrapper, wrapper
}
