#!/bin/sh
# Example:  ./hack/go-lint.sh installer/... pkg/... tests/smoke

REPO_NAME=$(basename "${PWD}")
if [ "$IS_CONTAINER" != "" ]; then
  golint -set_exit_status "${@}"
else
  docker run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/openshift/${REPO_NAME}:z" \
    --workdir "/go/src/github.com/openshift/${REPO_NAME}" \
    --env GO111MODULE="$GO111MODULE" \
    --env GOFLAGS="$GOFLAGS" \
    openshift/origin-release:golang-1.16 \
    ./hack/go-lint.sh "${@}"
fi
