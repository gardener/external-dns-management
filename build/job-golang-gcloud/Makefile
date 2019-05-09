VERSION=$(shell cat ../../VERSION)

.PHONY: build push


build:
	docker build . -t job-golang-gcloud


push:
	docker tag job-golang-gcloud:latest eu.gcr.io/gardener-project/test/job-golang-gcloud:$(VERSION)
	docker tag job-golang-gcloud:latest eu.gcr.io/gardener-project/test/job-golang-gcloud:latest
	docker push eu.gcr.io/gardener-project/test/job-golang-gcloud:latest
	docker push eu.gcr.io/gardener-project/test/job-golang-gcloud:$(VERSION)
