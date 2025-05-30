// Copyright (c) 2016, 2018, 2024, Oracle and/or its affiliates.  All rights reserved.
// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.
// Code generated. DO NOT EDIT.

// Core Services API
//
// Use the Core Services API to manage resources such as virtual cloud networks (VCNs),
// compute instances, and block storage volumes. For more information, see the console
// documentation for the Networking (https://docs.cloud.oracle.com/iaas/Content/Network/Concepts/overview.htm),
// Compute (https://docs.cloud.oracle.com/iaas/Content/Compute/Concepts/computeoverview.htm), and
// Block Volume (https://docs.cloud.oracle.com/iaas/Content/Block/Concepts/overview.htm) services.
// The required permissions are documented in the
// Details for the Core Services (https://docs.cloud.oracle.com/iaas/Content/Identity/Reference/corepolicyreference.htm) article.
//

package core

import (
	"encoding/json"
	"fmt"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/oci/vendor-internal/github.com/oracle/oci-go-sdk/v65/common"
	"strings"
)

// UpdateInstancePlatformConfig The platform configuration to be updated for the instance.
type UpdateInstancePlatformConfig interface {
}

type updateinstanceplatformconfig struct {
	JsonData []byte
	Type     string `json:"type"`
}

// UnmarshalJSON unmarshals json
func (m *updateinstanceplatformconfig) UnmarshalJSON(data []byte) error {
	m.JsonData = data
	type Unmarshalerupdateinstanceplatformconfig updateinstanceplatformconfig
	s := struct {
		Model Unmarshalerupdateinstanceplatformconfig
	}{}
	err := json.Unmarshal(data, &s.Model)
	if err != nil {
		return err
	}
	m.Type = s.Model.Type

	return err
}

// UnmarshalPolymorphicJSON unmarshals polymorphic json
func (m *updateinstanceplatformconfig) UnmarshalPolymorphicJSON(data []byte) (interface{}, error) {

	if data == nil || string(data) == "null" {
		return nil, nil
	}

	var err error
	switch m.Type {
	case "AMD_VM":
		mm := AmdVmUpdateInstancePlatformConfig{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	case "INTEL_VM":
		mm := IntelVmUpdateInstancePlatformConfig{}
		err = json.Unmarshal(data, &mm)
		return mm, err
	default:
		common.Logf("Recieved unsupported enum value for UpdateInstancePlatformConfig: %s.", m.Type)
		return *m, nil
	}
}

func (m updateinstanceplatformconfig) String() string {
	return common.PointerString(m)
}

// ValidateEnumValue returns an error when providing an unsupported enum value
// This function is being called during constructing API request process
// Not recommended for calling this function directly
func (m updateinstanceplatformconfig) ValidateEnumValue() (bool, error) {
	errMessage := []string{}

	if len(errMessage) > 0 {
		return true, fmt.Errorf(strings.Join(errMessage, "\n"))
	}
	return false, nil
}

// UpdateInstancePlatformConfigTypeEnum Enum with underlying type: string
type UpdateInstancePlatformConfigTypeEnum string

// Set of constants representing the allowable values for UpdateInstancePlatformConfigTypeEnum
const (
	UpdateInstancePlatformConfigTypeAmdVm   UpdateInstancePlatformConfigTypeEnum = "AMD_VM"
	UpdateInstancePlatformConfigTypeIntelVm UpdateInstancePlatformConfigTypeEnum = "INTEL_VM"
)

var mappingUpdateInstancePlatformConfigTypeEnum = map[string]UpdateInstancePlatformConfigTypeEnum{
	"AMD_VM":   UpdateInstancePlatformConfigTypeAmdVm,
	"INTEL_VM": UpdateInstancePlatformConfigTypeIntelVm,
}

var mappingUpdateInstancePlatformConfigTypeEnumLowerCase = map[string]UpdateInstancePlatformConfigTypeEnum{
	"amd_vm":   UpdateInstancePlatformConfigTypeAmdVm,
	"intel_vm": UpdateInstancePlatformConfigTypeIntelVm,
}

// GetUpdateInstancePlatformConfigTypeEnumValues Enumerates the set of values for UpdateInstancePlatformConfigTypeEnum
func GetUpdateInstancePlatformConfigTypeEnumValues() []UpdateInstancePlatformConfigTypeEnum {
	values := make([]UpdateInstancePlatformConfigTypeEnum, 0)
	for _, v := range mappingUpdateInstancePlatformConfigTypeEnum {
		values = append(values, v)
	}
	return values
}

// GetUpdateInstancePlatformConfigTypeEnumStringValues Enumerates the set of values in String for UpdateInstancePlatformConfigTypeEnum
func GetUpdateInstancePlatformConfigTypeEnumStringValues() []string {
	return []string{
		"AMD_VM",
		"INTEL_VM",
	}
}

// GetMappingUpdateInstancePlatformConfigTypeEnum performs case Insensitive comparison on enum value and return the desired enum
func GetMappingUpdateInstancePlatformConfigTypeEnum(val string) (UpdateInstancePlatformConfigTypeEnum, bool) {
	enum, ok := mappingUpdateInstancePlatformConfigTypeEnumLowerCase[strings.ToLower(val)]
	return enum, ok
}
