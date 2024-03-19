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

## Using the API Token

Use the token in a `Secret` resource with the `metadata.name` to be 
`cloudflare-credentials` and `data.CLOUDFLARE_API_TOKEN` to be the token.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: default
type: Opaque
data:
  CLOUDFLARE_API_TOKEN: 1234567890123456789
``` 

## Troubleshooting

* If you get a permission error communicating with Cloudflare, be sure the domain name 
  being registered does not exceed your plan limits. Hierarchical domains are not
  supported on the free plan as of this writing.
