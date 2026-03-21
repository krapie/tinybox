FROM golang:1.23-alpine AS builder
# GOTOOLCHAIN=auto lets the Go toolchain download the version declared in go.mod
# if it is newer than the one bundled in the image.
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /tinykube ./cmd/tinykube/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /tinykube /usr/local/bin/tinykube
EXPOSE 8080
ENTRYPOINT ["tinykube"]
