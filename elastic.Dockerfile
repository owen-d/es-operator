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

RUN gpg --keyserver pool.sks-keyservers.net --recv-keys B42F6819007F00F88E364FD4036A9C25BF357DD4 \
&& curl -o /usr/local/bin/gosu -SL "https://github.com/tianon/gosu/releases/download/1.2/gosu-amd64" \
&& curl -o /usr/local/bin/gosu.asc -SL "https://github.com/tianon/gosu/releases/download/1.2/gosu-amd64.asc" \
&& gpg --verify /usr/local/bin/gosu.asc \
&& rm /usr/local/bin/gosu.asc \
&& rm -r /root/.gnupg/ \
&& chmod +x /usr/local/bin/gosu

COPY binutils/wrapper.sh /usr/local/bin/wrapper
RUN chmod +x /usr/local/bin/wrapper

COPY --from=builder /go/src/github.com/owen-d/es-operator/handler /usr/local/bin/handler

# USER elasticsearch

ENTRYPOINT ["/usr/local/bin/wrapper"]
CMD ["eswrapper"]
