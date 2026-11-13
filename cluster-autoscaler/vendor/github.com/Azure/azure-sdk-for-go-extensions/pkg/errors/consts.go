package errors

const (

	// Error codes
	OperationNotAllowed                   = "OperationNotAllowed"
	AllocationFailed                      = "AllocationFailed"
	OverconstrainedAllocationRequest      = "OverconstrainedAllocationRequest"
	ZoneAllocationFailed                  = "ZonalAllocationFailed"
	OverconstrainedZonalAllocationRequest = "OverconstrainedZonalAllocationRequest"
	NicReservedForAnotherVM               = "NicReservedForAnotherVm"
	SKUNotAvailableErrorCode              = "SkuNotAvailable"
	InsufficientSubnetSizeErrorCode       = "InsufficientSubnetSize"

	// Error search terms
	LowPriorityQuotaExceededTerm  = "LowPriorityCores"
	SKUFamilyQuotaExceededTerm    = "Family Cores quota"
	SubscriptionQuotaExceededTerm = "Submit a request for Quota increase"
	RegionalQuotaExceededTerm     = "exceeding approved Total Regional Cores quota"
)
