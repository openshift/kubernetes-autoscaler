#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

function cleanup() {
    return_code=$?
    os::util::describe_return_code "${return_code}"
    exit "${return_code}"
}
trap "cleanup" EXIT

os::golang::verify_go_version

bad_files=$(os::util::list_go_src_files | xargs gofmt -s -l)
if [[ -n "${bad_files}" ]]; then
  echo "Please run hack/update-gofmt.sh to fix the following files:"
  echo "${bad_files}"
  exit 1
fi
