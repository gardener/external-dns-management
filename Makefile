EXECUTABLE=dns-controller-manager
PROJECT=github.com/mandelsoft/dns-controller-manager
VERSION=$(shell cat VERSION)


.PHONY: build release


build:
	GOOS=linux GOARCH=amd64 go build -o $(EXECUTABLE) \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/dns


release:
	GOOS=linux GOARCH=amd64 go build -o $(EXECUTABLE) \
	    -ldflags "-X main.Version=$(VERSION) \
	    ./cmd/dns
