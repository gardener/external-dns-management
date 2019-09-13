EXECUTABLE=dns-controller-manager
PROJECT=github.com/gardener/external-dns-management
VERSION=$(shell cat VERSION)


.PHONY: build local-build release test alltests


build:
	GOOS=linux GOARCH=amd64 go build -o $(EXECUTABLE) \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/dns

local-build:
	go build -o $(EXECUTABLE) \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/dns

release:
	GOOS=linux GOARCH=amd64 go build -o $(EXECUTABLE) \
	    -ldflags "-X main.Version=$(VERSION) \
	    ./cmd/dns

test:
	go test ./pkg/...
	@echo ----- Skipping long running integration tests, use \'make alltests\' to run all tests -----
	test/integration/run.sh $(kindargs) -- -skip Many $(args)

alltests:
	go test ./pkg/...
	test/integration/run.sh $(kindargs) -- $(args)
