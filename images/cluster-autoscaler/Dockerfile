FROM registry.svc.ci.openshift.org/openshift/release:golang-1.13 AS builder
WORKDIR /go/src/k8s.io/autoscaler
COPY . .
RUN go build -o cluster-autoscaler/cluster-autoscaler ./cluster-autoscaler

CMD /usr/bin/cluster-autoscaler
