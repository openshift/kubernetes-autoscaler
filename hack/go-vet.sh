#!/bin/sh
REPO_NAME=$(basename "${PWD}")
if [ "$IS_CONTAINER" != "" ]; then
  go vet "${@}"
else
  docker run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/k8s.io/autoscaler:z" \
    --workdir "/go/src/k8s.io/autoscaler" \
    openshift/origin-release:golang-1.10 \
    ./hack/go-vet.sh "${@}"
fi
