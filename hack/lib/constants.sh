#!/bin/bash

# This script provides constants for the Golang binary build process

readonly OS_GO_PACKAGE=k8s.io/autoscaler

readonly OS_BUILD_ENV_GOLANG="${OS_BUILD_ENV_GOLANG:-1.9}"
readonly OS_BUILD_ENV_IMAGE="${OS_BUILD_ENV_IMAGE:-openshift/origin-release:golang-${OS_BUILD_ENV_GOLANG}}"
readonly OS_REQUIRED_GO_VERSION="go1.9"
readonly OS_BUILD_ENV_WORKINGDIR="/go/${OS_GO_PACKAGE}"

readonly OS_OUTPUT_BASEPATH="${OS_OUTPUT_BASEPATH:-_output}"
readonly OS_BASE_OUTPUT="${OS_ROOT}/${OS_OUTPUT_BASEPATH}"
readonly OS_OUTPUT_SCRIPTPATH="${OS_OUTPUT_SCRIPTPATH:-"${OS_BASE_OUTPUT}/scripts"}"

readonly OS_OUTPUT_SUBPATH="${OS_OUTPUT_SUBPATH:-${OS_OUTPUT_BASEPATH}/local}"
readonly OS_OUTPUT="${OS_ROOT}/${OS_OUTPUT_SUBPATH}"
readonly OS_OUTPUT_RELEASEPATH="${OS_OUTPUT}/releases"
readonly OS_OUTPUT_RPMPATH="${OS_OUTPUT_RELEASEPATH}/rpms"
readonly OS_OUTPUT_BINPATH="${OS_OUTPUT}/bin"
readonly OS_OUTPUT_PKGDIR="${OS_OUTPUT}/pkgdir"

readonly OS_GOFLAGS_TAGS="include_gcs include_oss containers_image_openpgp"

readonly OS_IMAGE_COMPILE_BINARIES=( )

readonly OS_CROSS_COMPILE_TARGETS=(
  cluster-autoscaler
)
readonly OS_CROSS_COMPILE_BINARIES=("${OS_CROSS_COMPILE_TARGETS[@]##*/}")

readonly OS_TEST_TARGETS=( )

# os::build::get_product_vars exports variables that we expect to change
# depending on the distribution of Origin
function os::build::get_product_vars() {
  export OS_BUILD_LDFLAGS_IMAGE_PREFIX="${OS_IMAGE_PREFIX:-"openshift/origin"}"
  export OS_BUILD_LDFLAGS_DEFAULT_IMAGE_STREAMS="${OS_BUILD_LDFLAGS_DEFAULT_IMAGE_STREAMS:-"centos7"}"
}

# os::build::ldflags calculates the -ldflags argument for building OpenShift
function os::build::ldflags() {
  # Run this in a subshell to prevent settings/variables from leaking.
  set -o errexit
  set -o nounset
  set -o pipefail

  cd "${OS_ROOT}"

  os::build::version::get_vars
  os::build::get_product_vars

  local buildDate="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

  declare -a ldflags=()

  ldflags+=($(os::build::ldflag "${OS_GO_PACKAGE}/pkg/oc/bootstrap/docker.defaultImageStreams" "${OS_BUILD_LDFLAGS_DEFAULT_IMAGE_STREAMS}"))
  ldflags+=($(os::build::ldflag "${OS_GO_PACKAGE}/pkg/cmd/util/variable.DefaultImagePrefix" "${OS_BUILD_LDFLAGS_IMAGE_PREFIX}"))
  ldflags+=($(os::build::ldflag "${OS_GO_PACKAGE}/pkg/version.majorFromGit" "${OS_GIT_MAJOR}"))
  ldflags+=($(os::build::ldflag "${OS_GO_PACKAGE}/pkg/version.minorFromGit" "${OS_GIT_MINOR}"))
  ldflags+=($(os::build::ldflag "${OS_GO_PACKAGE}/pkg/version.versionFromGit" "${OS_GIT_VERSION}"))
  ldflags+=($(os::build::ldflag "${OS_GO_PACKAGE}/pkg/version.commitFromGit" "${OS_GIT_COMMIT}"))
  ldflags+=($(os::build::ldflag "${OS_GO_PACKAGE}/pkg/version.buildDate" "${buildDate}"))

  # The -ldflags parameter takes a single string, so join the output.
  echo "${ldflags[*]-}"
}
readonly -f os::build::ldflags

# No-op
function os::build::generate_windows_versioninfo() {
  :
}
readonly -f os::build::generate_windows_versioninfo

function os::build::clean_windows_versioninfo() {
  :
}
readonly -f os::build::clean_windows_versioninfo

# os::util::list_go_src_files lists files we consider part of our project
# source code, useful for tools that iterate over source to provide vet-
# ting or linting, etc.
#
# Globals:
#  None
# Arguments:
#  None
# Returns:
#  None
function os::util::list_go_src_files() {
	find . -not \( \
		\( \
		-wholename './_output' \
		-o -wholename './.*' \
		-o -wholename '*/vendor/*' \
		\) -prune \
	\) -name '*.go' | sort -u
}
readonly -f os::util::list_go_src_files

# os::util::list_go_src_dirs lists dirs in origin/ and cmd/ dirs excluding
# doc.go useful for tools that iterate over source to provide vetting or 
# linting, or for godep-save etc.
#
# Globals:
#  None
# Arguments:
#  None
# Returns:
#  None
function os::util::list_go_src_dirs() {
	os::util::list_go_src_files | cut -d '/' -f 1-2 | grep -v ".go$" | grep -v "^./cmd" | LC_ALL=C sort -u
	os::util::list_go_src_files | grep "^./cmd/"| cut -d '/' -f 1-3 | grep -v ".go$" | LC_ALL=C sort -u
}
readonly -f os::util::list_go_src_dirs

# os::util::list_test_packages_under lists all packages containing Golang test files that we 
# want to run as unit tests under the given base dir in the source tree
function os::util::list_test_packages_under() {
    local basedir=$*

    # we do not quote ${basedir} to allow for multiple arguments to be passed in as well as to allow for
    # arguments that use expansion, e.g. paths containing brace expansion or wildcards
    find ${basedir} -not \(                   \
        \(                                    \
              -path 'vendor'                  \
              -o -path 'tools'                \
              -o -path 'tools'                \
              -o -path '*_output'             \
              -o -path '*.git'                \
              -o -path '*vendor/*'            \
              -o -path 'addon-resizer'        \
              -o -path 'vertical-pod-autoscaler' \
              -o -path 'cluster-autoscaler/cloudprovider/alicloud' \
              -o -path 'cluster-autoscaler/cloudprovider/aws' \
              -o -path 'cluster-autoscaler/cloudprovider/azure' \
              -o -path 'cluster-autoscaler/cloudprovider/baiducloud' \
              -o -path 'cluster-autoscaler/cloudprovider/bizflycloud' \
              -o -path 'cluster-autoscaler/cloudprovider/brightbox' \
              -o -path 'cluster-autoscaler/cloudprovider/cherryservers' \
              -o -path 'cluster-autoscaler/cloudprovider/civo' \
              -o -path 'cluster-autoscaler/cloudprovider/cloudstack' \
              -o -path 'cluster-autoscaler/cloudprovider/digitalocean' \
              -o -path 'cluster-autoscaler/cloudprovider/exoscale' \
              -o -path 'cluster-autoscaler/cloudprovider/externalgrpc' \
              -o -path 'cluster-autoscaler/cloudprovider/gce' \
              -o -path 'cluster-autoscaler/cloudprovider/hetzner' \
              -o -path 'cluster-autoscaler/cloudprovider/huaweicloud' \
              -o -path 'cluster-autoscaler/cloudprovider/ionoscloud' \
              -o -path 'cluster-autoscaler/cloudprovider/kamatera' \
              -o -path 'cluster-autoscaler/cloudprovider/kubemark' \
              -o -path 'cluster-autoscaler/cloudprovider/linode' \
              -o -path 'cluster-autoscaler/cloudprovider/magnum' \
              -o -path 'cluster-autoscaler/cloudprovider/mocks' \
              -o -path 'cluster-autoscaler/cloudprovider/oci' \
              -o -path 'cluster-autoscaler/cloudprovider/ovhcloud' \
              -o -path 'cluster-autoscaler/cloudprovider/packet' \
              -o -path 'cluster-autoscaler/cloudprovider/rancher' \
              -o -path 'cluster-autoscaler/cloudprovider/scaleway' \
              -o -path 'cluster-autoscaler/cloudprovider/tencentcloud' \
              -o -path 'cluster-autoscaler/cloudprovider/vultr' \
        \) -prune                             \
    \) -name '*_test.go' | xargs -n1 dirname | sort -u | xargs -n1 printf "${OS_GO_PACKAGE}/%s\n"
}
readonly -f os::util::list_test_packages_under

# os::util::list_go_deps outputs the list of dependencies for the project.
function os::util::list_go_deps() {
  go list -f '{{.ImportPath}}{{.Imports}}' ./cluster-autoscaler/... ./vertical-pod-autoscaler/... | tr '[]' '  ' | 
    sed -e 's|${OS_GO_PACKAGE}/vendor/||g'
}

# OS_ALL_IMAGES is the list of images built by os::build::images.
readonly OS_ALL_IMAGES=(
  origin-cluster-autoscaler
)

# os::build::images builds all images in this repo.
function os::build::images() {
  tag_prefix="${OS_IMAGE_PREFIX:-"openshift/origin"}"
  os::build::image "${tag_prefix}-cluster-autoscaler" images/cluster-autoscaler
}
