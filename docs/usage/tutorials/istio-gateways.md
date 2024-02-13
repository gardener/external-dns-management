# Using annotated Istio Gateway and/or Istio Virtual Service as Source
This tutorial describes how to use annotated Istio Gateway resources as source for DNSEntries in the dns-controller-manager.

## Install dns-controller-manager
First, install the dns-controller-manager similar as described in the [Quick Start](../../README.md#quick-start)

## Install Istio on your cluster

Follow the [Istio Getting Started](https://istio.io/latest/docs/setup/getting-started/) to download and install Istio.

These are the typical commands for the istio demo installation

```bash
export KUEBCONFIG=...
curl -L https://istio.io/downloadIstio | sh -
istioctl install --set profile=demo -y
kubectl label namespace default istio-injection=enabled
```

*Note: If you are using a KinD cluster, the istio-ingressgateway service may be pending forever.*

```bash
$ kubectl -n istio-system get svc istio-ingressgateway
NAME                   TYPE           CLUSTER-IP     EXTERNAL-IP   PORT(S)                                                                      AGE
istio-ingressgateway   LoadBalancer   10.96.88.189   <pending>     15021:30590/TCP,80:30185/TCP,443:30075/TCP,31400:30129/TCP,15443:30956/TCP   13m
```

In this case, you may patch the status for demo purposes (of course it still would not accept connections)
```bash
kubectl -n istio-system patch svc istio-ingressgateway --type=merge --subresource status --patch '{"status":{"loadBalancer":{"ingress":[{"ip":"1.2.3.4"}]}}}'
```

## Verify that Istio Gateway/VirtualService Source works

### Install a sample service
With automatic sidecar injection:
```bash
$ kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/httpbin/httpbin.yaml
```

### Using a Gateway as a source
#### Create an Istio Gateway:
```bash
$ cat <<EOF | kubectl apply -f -
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: httpbin-gateway
  namespace: istio-system
  annotations:
    dns.gardener.cloud/dnsnames: "*"
    #dns.gardener.cloud/class: garden # uncomment if used in a Gardener shoot cluster
spec:
  selector:
    istio: ingressgateway # use Istio default gateway implementation
  servers:
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
    - "httpbin.example.com" # this is used by the dns-controller-manager to extract DNS names
EOF
```

#### Configure routes for traffic entering via the Gateway:
```bash
$ cat <<EOF | kubectl apply -f -
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: httpbin
  namespace: default
spec:
  hosts:
  - "httpbin.example.com" # this is also used by the dns-controller-manager to extract DNS names
  gateways:
  - istio-system/httpbin-gateway
  http:
  - match:
    - uri:
        prefix: /status
    - uri:
        prefix: /delay
    route:
    - destination:
        port:
          number: 8000
        host: httpbin
EOF
```

You should now see a created `DNSEntry` similar to:

```bash
$ kubectl -n istio-system get dnse -oyaml
apiVersion: v1
items:
- apiVersion: dns.gardener.cloud/v1alpha1
  kind: DNSEntry
  metadata:
    generateName: httpbin-gateway-gateway-
    generation: 1
    name: httpbin-gateway-gateway-bwlh8
    namespace: default
    ownerReferences:
    - apiVersion: networking.istio.io/v1beta1
      blockOwnerDeletion: true
      controller: true
      kind: Gateway
      name: httpbin-gateway
      uid: 58dc81a0-9b18-46a1-8681-74a471e88d8e
  spec:
    dnsName: httpbin.example.com
    targets:
    - 1.2.3.4
kind: List
metadata:
  resourceVersion: ""
```

If it reports "No responsible provider found" in the status, you may not have set up a suitable DNSProvider, but
the demonstration here is about the creation of the `DNSEntry` itself.

### Using a VirtualService as a source

If the `Gateway` resource is annotated with `dns.gardener.cloud/dnsnames: "*"`, hosts from all referencing  `VirtualServices` resources
are automatically extracted. These resources don't need an additional annotation.

#### Create an Istio Gateway:
```bash
$ cat <<EOF | kubectl apply -f -
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: httpbin-gateway
  namespace: istio-system
  annotations:
    dns.gardener.cloud/dnsnames: "*"
    #dns.gardener.cloud/class: garden # uncomment if used in a Gardener shoot cluster
spec:
  selector:
    istio: ingressgateway # use Istio default gateway implementation
  servers:
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
    - "*"
EOF
```

#### Configure routes for traffic entering via the Gateway:
```bash
$ cat <<EOF | kubectl apply -f -
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: httpbin
  namespace: default  
spec:
  hosts:
  - "httpbin.example.com" # this is used by dns-controller-manager to extract DNS names
  gateways:
  - istio-system/httpbin-gateway
  http:
  - match:
    - uri:
        prefix: /status
    - uri:
        prefix: /delay
    route:
    - destination:
        port:
          number: 8000
        host: httpbin
EOF
```

This should show a similar DNSEntry as above.

To get the targets to the extracted DNS names, dns-controller-manager is able to gather information from the kubernetes service of the Istio Ingress Gateway.

**Note**: It is also possible to set the targets my specifying an Ingress resource using the `dns.gardener.cloud/ingress` annotation on the Istio Ingress Gateway resource.

**Note**: It is also possible to set the targets manually by using the `dns.gardener.cloud/targets` annotation on the Istio Ingress Gateway resource.

### Access the sample service using `curl`
```bash
$ curl -I http://httpbin.example.com/status/200
HTTP/1.1 200 OK
server: istio-envoy
date: Tue, 13 Feb 2024 07:49:37 GMT
content-type: text/html; charset=utf-8
access-control-allow-origin: *
access-control-allow-credentials: true
content-length: 0
x-envoy-upstream-service-time: 15
```

Accessing any other URL that has not been explicitly exposed should return an HTTP 404 error:
```bash
$ curl -I http://httpbin.example.com/headers
HTTP/1.1 404 Not Found
date: Tue, 13 Feb 2024 08:09:41 GMT
server: istio-envoy
transfer-encoding: chunked
```
