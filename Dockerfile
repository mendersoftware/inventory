FROM alpine:3.4

EXPOSE 8080

COPY ./inventory /usr/bin/

RUN mkdir /etc/inventory
COPY ./config.yaml /etc/inventory/

ENTRYPOINT ["/usr/bin/inventory", "-config", "/etc/inventory/config.yaml"]

RUN apk add --update ca-certificates && update-ca-certificates