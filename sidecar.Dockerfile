# Build the manager binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/owen-d/es-operator
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY handlers/ handlers/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o reloader github.com/owen-d/es-operator/cmd/reloader

# Copy the controller-manager into a thin image
FROM alpine:latest
WORKDIR /
COPY --from=builder /go/src/github.com/owen-d/es-operator/reloader .
ENTRYPOINT ["/reloader"]
