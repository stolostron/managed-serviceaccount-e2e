FROM registry.ci.openshift.org/stolostron/builder:go1.17-linux AS builder

ARG REMOTE_SOURCE
ARG REMOTE_SOURCE_DIR

COPY $REMOTE_SOURCE $REMOTE_SOURCE_DIR/app/
WORKDIR $REMOTE_SOURCE_DIR/app

# compile go tests in build image
RUN GOFLAGS="" go get -u github.com/onsi/ginkgo/v2/ginkgo@v2.1.0
RUN GOFLAGS="" ginkgo build pkg/tests/placeholder


FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
RUN microdnf update && \
    microdnf clean all

# expose env vars for runtime
ENV KUBECONFIG "/opt/.kube/config"
ENV IMPORT_KUBECONFIG "/opt/.kube/import-kubeconfig"
ENV OPTIONS "/resources/options.yaml"

# install ginkgo into built image
COPY --from=builder /go/bin/ /usr/local/bin

# install operator binary
COPY --from=builder $REMOTE_SOURCE_DIR/app/pkg/tests/placeholder/placeholder.test /test/placeholder/placeholder.test

VOLUME /results
WORKDIR "/test"

CMD ["/bin/bash", "-c", "ginkgo placeholder/placeholder.test -- --ginkgo.trace --ginkgo.v --ginkgo.junit-report=/results/result-managed-serviceaccount-placeholder.xml"]