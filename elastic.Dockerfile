FROM docker.elastic.co/elasticsearch/elasticsearch:6.6.1

WORKDIR /usr/share/elasticsearch
USER root

RUN curl -L https://github.com/progrium/entrykit/releases/download/v0.4.0/entrykit_0.4.0_Linux_x86_64.tgz | tar -xzvf - -C /usr/local/bin && /usr/local/bin/entrykit --symlink

COPY binutils/wrapper.sh /usr/local/bin/wrapper
RUN chmod +x /usr/local/bin/wrapper

USER elasticsearch

ENTRYPOINT ["codep", "handler", "/usr/local/bin/wrapper"]
CMD ["eswrapper"]
