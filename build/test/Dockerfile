FROM docker:stable-dind


# prepare golang

RUN apk add --no-cache git make musl-dev go bash curl

ENV GOROOT /usr/lib/go
ENV GOPATH /go
ENV PATH /go/bin:$PATH

RUN mkdir -p ${GOPATH}/src ${GOPATH}/bin

# install kubectl
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin

# install kind
RUN go get sigs.k8s.io/kind

# install ginkgo
RUN go get github.com/onsi/ginkgo/ginkgo && \
    go install github.com/onsi/ginkgo/ginkgo

WORKDIR $GOPATH

ENV DOCKER_IN_DOCKER=1
