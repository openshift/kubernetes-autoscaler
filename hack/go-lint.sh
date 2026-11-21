#!/bin/sh
# Example:  ./hack/go-lint.sh installer/... pkg/... tests/smoke

REPO_NAME=$(basename "${PWD}")
if [ "$IS_CONTAINER" != "" ]; then
  lint_dir=$(mktemp -d) || exit 1
  trap 'rm -rf "$lint_dir"' EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  (cd cluster-autoscaler && GO111MODULE=on go build -mod=vendor -o "$lint_dir/golint" ./vendor/golang.org/x/lint/golint) || exit "$?"
  "$lint_dir/golint" -set_exit_status "${@}"
else
  docker run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/openshift/${REPO_NAME}:z" \
    --workdir "/go/src/github.com/openshift/${REPO_NAME}" \
    --env GO111MODULE="$GO111MODULE" \
    --env GOFLAGS="$GOFLAGS" \
    registry.ci.openshift.org/openshift/release:golang-1.22 \
    ./hack/go-lint.sh "${@}"
fi
