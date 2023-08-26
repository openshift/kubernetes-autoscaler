/*
Copyright 2023 The Kubernetes Authors.

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

package cpu

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"sync"
	"testing"
)

func TestToSystemArchitecture(t *testing.T) {
	tcs := []struct {
		name     string
		archName string
		wantArch SystemArchitecture
	}{
		{
			name:     "valid architecture is converted",
			archName: "amd64",
			wantArch: Amd64,
		},
		{
			name:     "invalid architecture results in UnknownArchitecture",
			archName: "some-arch",
			wantArch: UnknownArch,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			gotArch := ToSystemArchitecture(tc.archName)
			if diff := cmp.Diff(tc.wantArch, gotArch); diff != "" {
				t.Errorf("ToSystemArchitecture diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetSystemArchitectureFromEnvOrDefault(t *testing.T) {
	amd64 := Amd64.Name()
	arm64 := Arm64.Name()
	wrongValue := "wrong"

	tcs := []struct {
		name     string
		envValue *string
		want     SystemArchitecture
	}{
		{
			name:     fmt.Sprintf("%s is set to arm64", systemArchitectureFlagValue),
			envValue: &arm64,
			want:     Arm64,
		},
		{
			name:     fmt.Sprintf("%s is set to amd64", systemArchitectureFlagValue),
			envValue: &amd64,
			want:     Amd64,
		},
		{
			name:     fmt.Sprintf("%s is not set", systemArchitectureFlagValue),
			envValue: nil,
			want:     DefaultArch,
		},
		{
			name:     fmt.Sprintf("%s is set to a wrong value", systemArchitectureFlagValue),
			envValue: &wrongValue,
			want:     DefaultArch,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// Reset the systemArchitecture variable to nil before each test due to the lazy initialization of the variable.
			systemArchitecture = nil
			systemArchitectureFlagValue = ""
			// Reset the once variable to its initial state before each test.
			once = sync.Once{}
			if tc.envValue != nil {
				systemArchitectureFlagValue = *tc.envValue
			}
			if got := GetDefaultScaleFromZeroArchitecture(); got != tc.want {
				t.Errorf("GetDefaultScaleFromZeroArchitecture() = %v, want %v", got, tc.want)
			}
		})
	}
}
