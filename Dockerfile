FROM golang:1.12.14-alpine3.9 as builder
RUN mkdir -p /go/src/github.com/mendersoftware/inventory
WORKDIR /go/src/github.com/mendersoftware/inventory
ADD ./ .
RUN CGO_ENABLED=0 GOARCH=amd64 go build -o inventory .

FROM alpine:3.9
EXPOSE 8080
RUN mkdir /etc/inventory
ENTRYPOINT ["/usr/bin/inventory", "--config", "/etc/inventory/config.yaml"]
COPY ./config.yaml /etc/inventory/
COPY --from=builder /go/src/github.com/mendersoftware/inventory/inventory /usr/bin/
RUN apk add --update ca-certificates && update-ca-certificates
