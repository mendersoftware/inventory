FROM golang:1.17.6-alpine3.15 as builder
WORKDIR /go/src/github.com/mendersoftware/inventory
RUN apk add --no-cache ca-certificates
COPY ./ .
RUN CGO_ENABLED=0 go build -o inventory .

FROM scratch
WORKDIR /etc/inventory
EXPOSE 8080
COPY ./config.yaml .
COPY --from=builder /go/src/github.com/mendersoftware/inventory/inventory /usr/bin/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/usr/bin/inventory", "--config", "/etc/inventory/config.yaml"]
