FROM golang:1.23-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /envoy ./cmd/envoy

FROM alpine:3.20
COPY --from=builder /envoy /usr/local/bin/envoy
ENTRYPOINT ["/usr/local/bin/envoy"]
CMD ["-config", "config.yaml"]
