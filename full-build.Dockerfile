FROM --platform=${BUILDPLATFORM} golang:1.27 AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG OPENSHIFT_BUILD_COMMIT

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN set -eux; \
    if [ -n "${OPENSHIFT_BUILD_COMMIT}" ]; then \
        export GIT_REVISION="${OPENSHIFT_BUILD_COMMIT}"; \
    fi; \
    TARGET_OS="${TARGETOS}" \
    TARGET_ARCH="${TARGETARCH}" \
    REPO_ROOT=/src \
    make --no-print-directory -f Makefile.core.mk build

FROM --platform=${BUILDPLATFORM} registry.access.redhat.com/ubi10/ubi:latest AS packager
ARG TARGETOS TARGETARCH

RUN dnf -y --setopt=install_weak_deps=0 --nodocs \
    --installroot /output install \
    setup \
 && dnf clean all --installroot /output
RUN [ -d /usr/share/buildinfo ] && cp -a /usr/share/buildinfo /output/usr/share/buildinfo ||:
RUN [ -d /root/buildinfo ] && cp -a /root/buildinfo /output/root/buildinfo ||:

FROM scratch
ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY --from=packager /output /
COPY --from=build /src/out/${TARGETOS}_${TARGETARCH}/sail-operator /sail-operator

USER 65532:65532
WORKDIR /
ENTRYPOINT ["/sail-operator"]
