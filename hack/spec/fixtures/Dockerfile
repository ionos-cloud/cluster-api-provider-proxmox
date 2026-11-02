FROM golang:1.25.0-trixie@sha256:abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd AS builder
WORKDIR /workspace
COPY . .
RUN go build -o manager .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/manager /manager
ENTRYPOINT ["/manager"]
