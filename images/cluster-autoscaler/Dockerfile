FROM openshift/origin-base

MAINTAINER Avesh Agarwal <avagarwa@redhat.com>

ENV container=docker

LABEL vendor="Red Hat, Inc." \
      com.redhat.component="origin-cluster-autoscaler" \
      name="openshift/origin-cluster-autoscaler" \
      version="1.2.0" \
      release="1" \
      architecture="x86_64" \
      summary="Cluster Autoscaler for OpenShift and Kubernetes"

RUN INSTALL_PKGS="origin-cluster-autoscaler" && \
    yum --enablerepo=origin-local-release install -y ${INSTALL_PKGS} && \
    rpm -V ${INSTALL_PKGS} && \
    yum clean all

CMD /usr/bin/cluster-autoscaler
