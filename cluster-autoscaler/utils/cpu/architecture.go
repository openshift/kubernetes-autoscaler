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
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
	"sync"
)

// SystemArchitecture represents a CPU architecture (e.g., amd64, arm64, ppc64le, s390x).
// It is used to determine the default architecture to use when building the nodes templates for scaling up from zero
// by some cloud providers. This code is the same as the GCE implementation at
// https://github.com/kubernetes/autoscaler/blob/3852f352d96b8763292a9122163c1152dfedec55/cluster-autoscaler/cloudprovider/gce/templates.go#L611-L657
// which is kept to allow for a smooth transition to this package, once the GCE team is ready to use it.
type SystemArchitecture string

const (
	// UnknownArch is used if the Architecture is Unknown
	UnknownArch SystemArchitecture = ""
	// Amd64 is used if the Architecture is x86_64
	Amd64 SystemArchitecture = "amd64"
	// Arm64 is used if the Architecture is ARM64
	Arm64 SystemArchitecture = "arm64"
	// Ppc64le is used if the Architecture is ppc64le
	Ppc64le SystemArchitecture = "ppc64le"
	// S390x is used if the Architecture is s390x
	S390x SystemArchitecture = "s390x"
	// DefaultArch should be used as a fallback if not passed by the environment via the --scale-up-from-zero-default-arch
	DefaultArch = Amd64
	// scaleUpFromZeroDefaultArchFlag is the flag name for the default architecture
	scaleUpFromZeroDefaultArchFlag = "scale-up-from-zero-default-arch"
)

var systemArchitecture *SystemArchitecture
var systemArchitectureFlagValue string
var once sync.Once

// GetDefaultScaleFromZeroArchitecture returns the SystemArchitecture from the flag --scale-up-from-zero-default-arch
// or DefaultArch if the variable is set to an invalid value.
// Cloud providers willing to opt into this implementation are expected to change their manager code by replacing
// the cloudprovider.DefaultArch constant with the GetDefaultScaleFromZeroArchitecture function exposed by this package.
// Usually, this should be done in the function responsible for generating the generic labels of the given
// cloud provider's manager code (for example, buildGenericLabels(.)) as follows:
//
//	func (...) buildGenericLabels(...) (map[string]string) {
//		result := make(map[string]string)
//		result[apiv1.LabelArchStable] = cpu.GetDefaultScaleFromZeroArchitecture().Name()
//		// ...
//	}
func GetDefaultScaleFromZeroArchitecture() SystemArchitecture {
	once.Do(func() {
		arch := ToSystemArchitecture(systemArchitectureFlagValue)
		klog.V(5).Infof("the --%s value is set to %s (%s)", scaleUpFromZeroDefaultArchFlag, systemArchitectureFlagValue, arch.Name())
		if arch == UnknownArch {
			arch = DefaultArch
			klog.Errorf("Unrecognized architecture '%s', falling back to %s",
				systemArchitectureFlagValue, DefaultArch.Name())
		}
		systemArchitecture = &arch
	})
	return *systemArchitecture
}

// ToSystemArchitecture parses a string to SystemArchitecture. Returns UnknownArch if the string doesn't represent a
// valid architecture.
func ToSystemArchitecture(arch string) SystemArchitecture {
	switch arch {
	case string(Arm64):
		return Arm64
	case string(Amd64):
		return Amd64
	case string(Ppc64le):
		return Ppc64le
	case string(S390x):
		return S390x
	default:
		return UnknownArch
	}
}

// Name returns the string value for SystemArchitecture
func (s SystemArchitecture) Name() string {
	return string(s)
}

// BindFlags binds the flags to the FlagSet set.
// Defining the flag here allows us to encapsulate the logic to parse the flag value, set the value of the
// systemArchitecture variable, and only expose the GetDefaultScaleFromZeroArchitecture function to the rest of the code.
func BindFlags(set *pflag.FlagSet) {
	set.StringVar(&systemArchitectureFlagValue, scaleUpFromZeroDefaultArchFlag, DefaultArch.Name(),
		"Default architecture to use when scaling up from zero. This is not supported by all the cloud providers. "+
			"Check your cloud provider's documentation. Valid values: [amd64, arm64, ppc64le, s390x]")
}
