FROM golang:1.11 as builder
ENV GO111MODULE=on 
RUN mkdir -p /go/src/github.com/mendersoftware/inventory
WORKDIR /go/src/github.com/mendersoftware/inventory
ADD ./ .
RUN go mod download
RUN CGO_ENABLED=0 GOARCH=amd64 go build -o inventory .

FROM alpine:3.4
EXPOSE 8080
COPY --from=builder /go/src/github.com/mendersoftware/inventory/inventory /usr/bin/
RUN mkdir /etc/inventory
COPY ./config.yaml /etc/inventory/
ENTRYPOINT ["/usr/bin/inventory", "--config", "/etc/inventory/config.yaml"]
RUN apk add --update ca-certificates && update-ca-certificates
