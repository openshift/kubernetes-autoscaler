/*
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

package errors

import (
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v9"
)

// Note: these functions should generally be kept in sync with armerrors.go counterparts
// An alternative is to expose a generic interface that works for both.
// Although, there seems to be no direct common interface, if not code/message extraction.

// extractErrorDetailDetails extracts code and message from ErrorDetail safely
func extractErrorDetailDetails(errorDetail armcontainerservice.ErrorDetail) (code, message string) {
	if errorDetail.Code != nil {
		code = *errorDetail.Code
	}
	if errorDetail.Message != nil {
		message = *errorDetail.Message
	}
	return code, message
}

// ZonalAllocationFailureOccurredInErrorDetail communicates if we have failed to allocate a resource in a zone, and should try another zone.
// To learn more about zonal allocation failures, visit: http://aka.ms/allocation-guidance
func ZonalAllocationFailureOccurredInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, _ := extractErrorDetailDetails(errorDetail)
	return isZonalAllocationFailed(code)
}

// AllocationFailureOccurredInErrorDetail communicates if we have failed to allocate a resource in a region, and should try another region.
func AllocationFailureOccurredInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, _ := extractErrorDetailDetails(errorDetail)
	return isAllocationFailed(code)
}

// OverconstrainedAllocationFailureOccurredInErrorDetail communicates if we have failed to allocate a resource that meets constraints specified in the request, and should try another region.
func OverconstrainedAllocationFailureOccurredInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, _ := extractErrorDetailDetails(errorDetail)
	return isOverconstrainedAllocationFailed(code)
}

// OverconstrainedZonalAllocationFailureOccurredInErrorDetail communicates if we have failed to allocate a resource that meets constraints specified in the request, and should try another zone.
func OverconstrainedZonalAllocationFailureOccurredInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, _ := extractErrorDetailDetails(errorDetail)
	return isOverconstrainedZonalAllocationFailed(code)
}

// SKUFamilyQuotaHasBeenReachedInErrorDetail tells us if we have exceeded our Quota.
func SKUFamilyQuotaHasBeenReachedInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, message := extractErrorDetailDetails(errorDetail)
	return isSKUFamilyQuotaExceeded(code, message)
}

// SubscriptionQuotaHasBeenReachedInErrorDetail tells us if we have exceeded our Quota.
func SubscriptionQuotaHasBeenReachedInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, message := extractErrorDetailDetails(errorDetail)
	return isSubscriptionQuotaExceeded(code, message)
}

// RegionalQuotaHasBeenReachedInErrorDetail communicates if we have reached the quota limit for a given region under a specific subscription
func RegionalQuotaHasBeenReachedInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, message := extractErrorDetailDetails(errorDetail)
	return isRegionalQuotaExceeded(code, message)
}

// LowPriorityQuotaHasBeenReachedInErrorDetail communicates if we have reached the quota limit for low priority VMs under a specific subscription
// Low priority VMs are generally Spot VMs, but can also be low priority VMs created via the Azure CLI or Azure Portal
func LowPriorityQuotaHasBeenReachedInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, message := extractErrorDetailDetails(errorDetail)
	return isLowPriorityQuotaExceeded(code, message)
}

// IsNicReservedForAnotherVMInErrorDetail occurs when a NIC is associated with another VM during deletion. See https://aka.ms/deletenic
func IsNicReservedForAnotherVMInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, _ := extractErrorDetailDetails(errorDetail)
	return isNicReservedForVM(code)
}

// IsSKUNotAvailableInErrorDetail https://aka.ms/azureskunotavailable: either not available for a location or zone, or out of capacity for Spot.
func IsSKUNotAvailableInErrorDetail(errorDetail armcontainerservice.ErrorDetail) bool {
	code, _ := extractErrorDetailDetails(errorDetail)
	return isSKUNotAvailable(code)
}

// IsInsufficientSubnetSizeErrorDetails occurs when a subnet that's in use for an AKS cluster no longer has available IP addresses
// for successful resource assignment. See http://aka.ms/aks/insufficientsubnetsize
func IsInsufficientSubnetSizeErrorDetails(errorDetail armcontainerservice.ErrorDetail) bool {
	code, _ := extractErrorDetailDetails(errorDetail)
	return isInsufficientSubnetSize(code)
}
