#!/bin/sh

# This will just symlink github.com/kubernetes-incubator/cluster-capacity -> github.com/openshift/cluster-capacity

mkdir -p _output/local/go/src/k8s.io/
ln -s $GOPATH/src/github.com/openshift/kubernetes-autoscaler _output/local/go/src/k8s.io/autoscaler

