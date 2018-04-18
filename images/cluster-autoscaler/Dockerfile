FROM rhel7:7-released

MAINTAINER Avesh Agarwal <avagarwa@redhat.com>

ENV container=docker

LABEL vendor="Red Hat, Inc." \
      com.redhat.component="atomic-openshift-cluster-autoscaler" \
      name="openshift3/ose-cluster-autoscaler" \
      version="1.2.0" \
      release="1" \
      architecture="x86_64" \
      summary="Cluster Autoscaler for OpenShift and Kubernetes"

RUN yum -y update && \
    yum -y install atomic-openshift-cluster-autoscaler && \
    yum clean all

CMD /usr/bin/cluster-autoscaler
