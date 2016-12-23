FROM alpine:3.4

COPY ./inventory /usr/bin/

RUN mkdir /etc/inventory
COPY ./config.yaml /etc/inventory/

ENTRYPOINT ["/usr/bin/inventory", "-config", "/etc/inventory/config.yaml"]
