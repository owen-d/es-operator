# Build the handler binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/owen-d/es-operator
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY handlers/ handlers/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o handler github.com/owen-d/es-operator/cmd/handler

# Copy the handler into a thin image
FROM docker.elastic.co/elasticsearch/elasticsearch:6.6.1
WORKDIR /usr/share/elasticsearch
USER root

RUN curl -L https://github.com/progrium/entrykit/releases/download/v0.4.0/entrykit_0.4.0_Linux_x86_64.tgz | tar -xzvf - -C /usr/local/bin && /usr/local/bin/entrykit --symlink

COPY binutils/wrapper.sh /usr/local/bin/wrapper
RUN chmod +x /usr/local/bin/wrapper

COPY --from=builder /go/src/github.com/owen-d/es-operator/handler /usr/local/bin/handler

USER elasticsearch

ENTRYPOINT ["/usr/local/bin/wrapper"]
CMD ["eswrapper"]
