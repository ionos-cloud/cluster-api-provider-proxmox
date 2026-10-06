# Build the manager binary.
# --platform=$BUILDPLATFORM pins the builder stage to the host architecture so
# that `docker buildx build --platform linux/amd64,linux/arm64` cross-compiles
# with the native Go toolchain instead of emulating the compiler under QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

# Build
# GOARCH is taken from TARGETARCH, which BuildKit sets per target platform. It has no default so that a
# plain `docker build` on an Apple Silicon host produces a linux/arm64 binary and on an x86 host a
# linux/amd64 one, i.e. the binary always matches the platform of the image it ships in.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
