FROM golang:1.11 as builder
ENV GO111MODULE=on
RUN mkdir -p /build
WORKDIR /build
ADD ./vendor /build/vendor
RUN go mod download
ADD . /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /build/inventory .

FROM alpine:3.4
EXPOSE 8080
COPY --from=builder /build/inventory /usr/bin/
RUN mkdir /etc/inventory
COPY ./config.yaml /etc/inventory/
ENTRYPOINT ["/usr/bin/inventory", "--config", "/etc/inventory/config.yaml"]
RUN apk add --update ca-certificates && update-ca-certificates
