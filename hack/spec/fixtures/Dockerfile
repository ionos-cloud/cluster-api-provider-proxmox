FROM golang:1.25 AS builder
WORKDIR /workspace
COPY . .
RUN go build -o manager .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/manager /manager
ENTRYPOINT ["/manager"]
