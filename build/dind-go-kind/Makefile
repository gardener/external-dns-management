VERSION=$(shell cat ../../VERSION)

.PHONY: build push


build:
	docker build . -t dind-go-kind


push:
	docker tag dind-go-kind:latest eu.gcr.io/gardener-project/test/dind-go-kind:$(VERSION)
	docker tag dind-go-kind:latest eu.gcr.io/gardener-project/test/dind-go-kind:latest
	docker push eu.gcr.io/gardener-project/test/dind-go-kind:latest
	docker push eu.gcr.io/gardener-project/test/dind-go-kind:$(VERSION)
