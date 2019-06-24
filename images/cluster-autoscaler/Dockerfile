FROM registry.svc.ci.openshift.org/openshift/release:golang-1.12 AS builder
WORKDIR /go/src/k8s.io/autoscaler
COPY . .
RUN go build -o cluster-autoscaler/cluster-autoscaler ./cluster-autoscaler

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base
COPY --from=builder /go/src/k8s.io/autoscaler/cluster-autoscaler/cluster-autoscaler /usr/bin/
CMD /usr/bin/cluster-autoscaler
LABEL summary="Cluster Autoscaler for OpenShift and Kubernetes"
