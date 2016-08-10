# Inventory
[![Build Status](https://travis-ci.com/mendersoftware/inventory.svg?token=rx8YqsZ2ZyaopcMPmDmo&branch=master)](https://travis-ci.com/mendersoftware/inventory)


Microservice for managing inventory data for IIoT devices within Mender ecosystem.

## Installation

Install instructions.

### Binaries (Linux x64)

Latest build of binaries for Linux 64bit are [available](LINK).

```
    wget <link>
```
    
### Docker Image

Prebuild docker images are available for each branch and tag. Available via [docker hub](https://hub.docker.com/r/mendersoftware/deployments/)

```
    docker pull mendersoftware/inventory:latest
    docke run -p 8080:8080 mendersoftware/inventory:latest 
```

### Source

Golang toolchain is required to build the application. [Installation instructions.](https://golang.org/doc/install)

Build instructions:

```
$ go get -u github.com/mendersoftware/inventory
$ cd $GOPATH/src/github.com/mendersoftware/inventory
$ go build
$ go test $(go list ./... | grep -v vendor)
```

Dependencies are managed using golang vendoring (GOVENDOREXPERIMENT)
