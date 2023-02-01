FROM golang:1.19.5-alpine3.16 as builder
WORKDIR /go/src/github.com/mendersoftware/inventory
RUN mkdir -p /etc_extra
RUN echo "nobody:x:65534:" > /etc_extra/group
RUN echo "nobody:!::0:::::" > /etc_extra/shadow
RUN echo "nobody:x:65534:65534:Nobody:/:" > /etc_extra/passwd
RUN chown -R nobody:nobody /etc_extra
RUN apk add --no-cache ca-certificates
COPY ./ .
RUN CGO_ENABLED=0 go build -o inventory .

FROM scratch
EXPOSE 8080
COPY --from=builder /etc_extra/ /etc/
USER 65534
WORKDIR /etc/inventory
COPY --from=builder --chown=nobody /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --chown=nobody ./config.yaml .
COPY --from=builder --chown=nobody /go/src/github.com/mendersoftware/inventory/inventory /usr/bin/
ENTRYPOINT ["/usr/bin/inventory", "--config", "/etc/inventory/config.yaml"]
