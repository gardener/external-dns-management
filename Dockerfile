FROM golang:1.12.8
WORKDIR /go/src/github.com/gardener/external-dns-management/
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build \
  -mod=vendor \
  -a \
  -o dns-controller-manager \
  -ldflags "-X main.Version=$(cat VERSION)-$(git rev-parse HEAD)" \
  ./cmd/dns

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /go/src/github.com/gardener/external-dns-management/dns-controller-manager .
ENTRYPOINT ["./dns-controller-manager"]
