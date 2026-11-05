#!/bin/sh

set -eu

export IS_CONTAINER=TRUE
export GO111MODULE=off

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

printf 'package linttest\n\nvar bad_name = 1\n' > "$test_dir/first file.go"
printf 'package linttest\n\nvar other_name = 2\n' > "$test_dir/second file.go"

./hack/go-lint.sh -min_confidence 1 "$test_dir/first file.go" "$test_dir/second file.go"

status=0
./hack/go-lint.sh -min_confidence 0.9 "$test_dir/first file.go" "$test_dir/second file.go" > "$test_dir/output" 2>&1 || status=$?
if [ "$status" -ne 1 ]; then
  echo 'expected lint violations to fail' >&2
  exit 1
fi
grep -F 'first file.go:3:5: don' "$test_dir/output"
grep -F 'second file.go:3:5: don' "$test_dir/output"

./hack/go-lint.sh -min_confidence 0.9 ./cluster-autoscaler/cloudprovider/clusterapi/...

status=0
./hack/go-lint.sh -invalid_lint_flag > "$test_dir/output" 2>&1 || status=$?
if [ "$status" -ne 2 ]; then
  echo 'expected an invalid flag to fail' >&2
  exit 1
fi
grep -F 'flag provided but not defined: -invalid_lint_flag' "$test_dir/output"

echo 'golint wrapper regression checks passed'
