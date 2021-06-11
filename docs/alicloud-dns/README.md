# Alicloud DNS Provider

This DNS provider allows you to create and manage DNS entries in [Alibaba Cloud DNS](https://www.alibabacloud.com/product/dns). 

## Generate New Access Key

You need to provide an access key (access key ID and secret access key) for Alibaba Cloud to allow the dns-controller-manager to 
authenticate to Alibaba Cloud DNS.

For details see [AccessKey Client](https://github.com/aliyun/alibaba-cloud-sdk-go/blob/master/docs/2-Client-EN.md#accesskey-client).
Currently the `regionId` is fixed to `cn-shanghai`. 

## Using the Access Key

Create a `Secret` resource with the data fields `ACCESS_KEY_ID` and `SECRET_ACCESS_KEY`.
The values are the base64 encoded access key ID and secret access key respectively.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: alicloud-credentials
  namespace: default
type: Opaque
data:
  # Replace '...' with values encoded as base64.
  ACCESS_KEY_ID: ...
  SECRET_ACCESS_KEY: ...

  # Alternatively use Gardener cloud provider credentials convention
  #accessKeyID: ...
  #secretAccessKey: ...
``` 
