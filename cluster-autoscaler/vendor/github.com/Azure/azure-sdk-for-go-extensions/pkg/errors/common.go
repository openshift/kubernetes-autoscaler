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

import "strings"

// Core matching logic - single source of truth
func isLowPriorityQuotaExceeded(code, message string) bool {
	return code == OperationNotAllowed && strings.Contains(message, LowPriorityQuotaExceededTerm)
}

func isSKUFamilyQuotaExceeded(code, message string) bool {
	return code == OperationNotAllowed && strings.Contains(message, SKUFamilyQuotaExceededTerm)
}

func isSubscriptionQuotaExceeded(code, message string) bool {
	return code == OperationNotAllowed && strings.Contains(message, SubscriptionQuotaExceededTerm)
}

func isRegionalQuotaExceeded(code, message string) bool {
	return code == OperationNotAllowed && strings.Contains(message, RegionalQuotaExceededTerm)
}

func isZonalAllocationFailed(code string) bool {
	return code == ZoneAllocationFailed
}

func isAllocationFailed(code string) bool {
	return code == AllocationFailed
}

func isOverconstrainedAllocationFailed(code string) bool {
	return code == OverconstrainedAllocationRequest
}

func isOverconstrainedZonalAllocationFailed(code string) bool {
	return code == OverconstrainedZonalAllocationRequest
}

func isSKUNotAvailable(code string) bool {
	return code == SKUNotAvailableErrorCode
}

func isInsufficientSubnetSize(code string) bool {
	return code == InsufficientSubnetSizeErrorCode
}

func isNicReservedForVM(code string) bool {
	return code == NicReservedForAnotherVM
}
