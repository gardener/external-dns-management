# Using annotated Gateway API Gateway and/or HTTPRoutes as Source
This tutorial describes how to use annotated Gateway API resources as source for DNSEntries in the dns-controller-manager.

The dns-controller-manager supports the resources `Gateway` and `HTTPRoute`. 

## Install dns-controller-manager
First, install the dns-controller-manager similar as described in the [Quick Start](../../README.md#quick-start)

## Install Istio on your cluster

Follow the Istio [Kubernetes Gateway API](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/) to 
install the Gateway API and to install Istio.

These are the typical commands for the Istio installation with the Kubernetes Gateway API:

```bash
export KUEBCONFIG=...
curl -L https://istio.io/downloadIstio | sh -
kubectl get crd gateways.gateway.networking.k8s.io &> /dev/null || \
  { kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v1.0.0" | kubectl apply -f -; }
istioctl install --set profile=minimal -y
kubectl label namespace default istio-injection=enabled
```

## Verify that Gateway Source works

### Install a sample service
With automatic sidecar injection:
```bash
$ kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/httpbin/httpbin.yaml
```

### Using a Gateway as a source

Deploy the Gateway API configuration including a single exposed route (i.e., /get):
```bash
kubectl create namespace istio-ingress
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: gateway
  namespace: istio-ingress
  annotations:
    dns.gardener.cloud/dnsnames: "*"
spec:
  gatewayClassName: istio
  listeners:
  - name: default
    hostname: "*.example.com"  # this is used by dns-controller-manager to extract DNS names
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
---
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: http
  namespace: default
spec:
  parentRefs:
  - name: gateway
    namespace: istio-ingress
  hostnames: ["httpbin.example.com"]  # this is used by dns-controller-manager to extract DNS names too
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /get
    backendRefs:
    - name: httpbin
      port: 8000
EOF
```

Check that the `Gateway` resource has addresses in its status, e.g.:
```bash
$ kubectl -n istio-ingress get gateway gateway -ojsonpath='{.status.addresses}'
[{"type":"IPAddress","value":"2.2.2.2"}]
```

*Note: If you are using a KinD cluster, the gateway will not receive addresses. In this case you can simulate the address assignment with*
```bash
kubectl -n istio-ingress patch gateway gateway --type=merge --subresource status --patch '{"status":{"addresses":[{"value":"2.2.2.2"}]}}'
```

You should now see a created `DNSEntry` similar to:

```bash
$ kubectl -n istio-system get dnse -oyaml
apiVersion: v1
items:
- apiVersion: dns.gardener.cloud/v1alpha1
  kind: DNSEntry
  metadata:
    generateName: gateway-gateway-
    name: gateway-gateway-4chzq
    namespace: istio-ingress
    ownerReferences:
    - apiVersion: gateway.networking.k8s.io/v1
      blockOwnerDeletion: true
      controller: true
      kind: Gateway
      name: gateway
      uid: 8ad90e0d-0f87-4d88-922c-7d511a556278
  spec:
    dnsName: '*.example.com'
    targets:
    - 2.2.2.2
kind: List
metadata:
  resourceVersion: ""
```

If it reports "No responsible provider found" in the status, you may not have set up a suitable DNSProvider, but
the demonstration here is about the creation of the `DNSEntry` itself.

#### Using a HTTPRoute as a source

If the `Gateway` resource is annotated with `dns.gardener.cloud/dnsnames: "*"`, hostnames from all referencing  `HTTPRoute` resources
are automatically extracted. These resources don't need an additional annotation.

Deploy the Gateway API configuration including a single exposed route (i.e., /get):

```bash
kubectl create namespace istio-ingress
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: gateway
  namespace: istio-ingress
  annotations:
    dns.gardener.cloud/dnsnames: "*"
spec:
  gatewayClassName: istio
  listeners:
  - name: default
    hostname: null  # not set 
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
---
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: http
  namespace: default
spec:
  parentRefs:
  - name: gateway
    namespace: istio-ingress
  hostnames: ["httpbin.example.com"]  # this is used by dns-controller-manager to extract DNS names too
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /get
    backendRefs:
    - name: httpbin
      port: 8000
EOF
```

This should show a similar DNSEntry as above.

#### Access the sample service using `curl`
```bash
$ curl -I http://httpbin.example.com/status/200
HTTP/1.1 200 OK
server: envoy
date: Tue, 28 Aug 2018 15:26:47 GMT
content-type: text/html; charset=utf-8
access-control-allow-origin: *
access-control-allow-credentials: true
content-length: 0
x-envoy-upstream-service-time: 5
```

Accessing any other URL that has not been explicitly exposed should return an HTTP 404 error:
```bash
$ curl -I http://httpbin.example.com/headers
HTTP/1.1 404 Not Found
date: Tue, 28 Aug 2018 15:27:48 GMT
server: envoy
transfer-encoding: chunked
```
