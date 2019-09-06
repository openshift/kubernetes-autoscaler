package nodegroupset

import (
	"math"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

const (
	// MaxMemoryDifferenceInKiloBytes describes how much memory
	// capacity can differ but still be considered equal.
	MaxMemoryDifferenceInKiloBytes = 2 << 7 * 1000 // 128Ki
)

// IsOpenShiftMachineApiNodeInfoSimilar compares if two nodes should
// be considered part of the same NodeGroupSet.
func IsOpenShiftMachineApiNodeInfoSimilar(n1, n2 *schedulernodeinfo.NodeInfo) bool {
	return isOpenShiftNodeInfoSimilar(n1, n2)
}

// Note: this is a copy of isNodeInfoSimilar() and the only change is
// to tolerate a small memory capacity difference.
func isOpenShiftNodeInfoSimilar(n1, n2 *schedulernodeinfo.NodeInfo) bool {
	capacity := make(map[apiv1.ResourceName][]resource.Quantity)
	allocatable := make(map[apiv1.ResourceName][]resource.Quantity)
	free := make(map[apiv1.ResourceName][]resource.Quantity)
	nodes := []*schedulernodeinfo.NodeInfo{n1, n2}
	for _, node := range nodes {
		for res, quantity := range node.Node().Status.Capacity {
			capacity[res] = append(capacity[res], quantity)
		}
		for res, quantity := range node.Node().Status.Allocatable {
			allocatable[res] = append(allocatable[res], quantity)
		}
		requested := node.RequestedResource()
		for res, quantity := range (&requested).ResourceList() {
			freeRes := node.Node().Status.Allocatable[res].DeepCopy()
			freeRes.Sub(quantity)
			free[res] = append(free[res], freeRes)
		}
	}

	for kind, qtyList := range capacity {
		if len(qtyList) != 2 {
			return false
		}
		switch kind {
		case apiv1.ResourceMemory:
			// For memory capacity we allow a small tolerance:
			//   https://bugzilla.redhat.com/show_bug.cgi?id=1731011
			//   https://bugzilla.redhat.com/show_bug.cgi?id=1733235
			larger := math.Max(float64(qtyList[0].Value()), float64(qtyList[1].Value()))
			smaller := math.Min(float64(qtyList[0].Value()), float64(qtyList[1].Value()))
			if larger-smaller > MaxMemoryDifferenceInKiloBytes {
				return false
			}
		default:
			// For other capacity types we require exact match.
			// If this is ever changed, enforcing MaxCoresTotal limits
			// as it is now may no longer work.
			if qtyList[0].Cmp(qtyList[1]) != 0 {
				return false
			}
		}
	}

	// For allocatable and free we allow resource quantities to be within a few % of each other
	if !compareResourceMapsWithTolerance(allocatable, MaxAllocatableDifferenceRatio) {
		return false
	}
	if !compareResourceMapsWithTolerance(free, MaxFreeDifferenceRatio) {
		return false
	}

	ignoredLabels := map[string]bool{
		apiv1.LabelHostname:                   true,
		apiv1.LabelZoneFailureDomain:          true,
		apiv1.LabelZoneRegion:                 true,
		"beta.kubernetes.io/fluentd-ds-ready": true, // this is internal label used for determining if fluentd should be installed as deamon set. Used for migration 1.8 to 1.9.
	}

	labels := make(map[string][]string)
	for _, node := range nodes {
		for label, value := range node.Node().ObjectMeta.Labels {
			ignore, _ := ignoredLabels[label]
			if !ignore {
				labels[label] = append(labels[label], value)
			}
		}
	}
	for _, labelValues := range labels {
		if len(labelValues) != 2 || labelValues[0] != labelValues[1] {
			return false
		}
	}
	return true
}
