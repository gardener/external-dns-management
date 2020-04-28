# Cloudflare DNS Provider

This DNS provider allows you to create and manage DNS entries in Cloudlfare. 

## Generate API Tokens

To use this provider you need to generate an API token from the Cloudflare dashboard.
A detailed documentation to generate an API token is available at 
https://support.cloudflare.com/hc/en-us/articles/200167836-Managing-API-Tokens-and-Keys.

**Note: You need to generate an API token and not an API key.**

To generate the token make sure the token has permission of Zone:Read and DNS:Edit for 
all zones. Optionally you can exclude certain zones.

**Note: You need to `Include` `All zones` in the `Zone Resources` section. Setting 
`Specific zone` doesn't work. But you can still add one or more `Exclude`s.**

![API token creation](api-token-creation.png)

Generate the token and keep this key safe as it won't be shown again.

Then base64 encode the token. For eg. if the generated token in `1234567890123456`, use

```bash
$ echo -n '1234567890123456789' | base64
```

to get the base64 encoded token. In this eg. this would be `MTIzNDU2Nzg5MDEyMzQ1Njc4OQ==`.

## Using the API Token

Use the base64 encoded token in a `Secret` resource with the `metadata.name` to be 
`cloudflare-credentials` and `data.CLOUDFLARE_API_TOKEN` to be the base64 encoded token.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: default
type: Opaque
data:
  CLOUDFLARE_API_TOKEN: MTIzNDU2Nzg5MDEyMzQ1Njc4OQ==
``` 