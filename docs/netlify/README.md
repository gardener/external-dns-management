# Netlify DNS Provider

This DNS provider allows you to create and manage DNS entries with [Netlify](https://www.netlify.com/). 

## Generate New Access Token

You need to provide an access token
for Netlify to allow the dns-controller-manager to authenticate to Netlify DNS API.

Then base64 encode the token. For eg. if the generated token in `1234567890123456`, use

```bash
$ echo -n '1234567890123456789' | base64
```

For details see https://docs.netlify.com/cli/get-started/#obtain-a-token-in-the-netlify-ui

## Required permissions

There are no special permissions for the access tokens.

## Using the Access Token

Create a `Secret` resource with the data fields `NETLIFY_AUTH_TOKEN`.
The value is the base64 encoded access token.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: netlify-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with values encoded as base64
  # see https://docs.netlify.com/cli/get-started/
  NETLIFY_AUTH_TOKEN: ...
  # Alternativly the key NETLIFY_API_TOKEN can be used
``` 