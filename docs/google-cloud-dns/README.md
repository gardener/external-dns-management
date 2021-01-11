# GCP Cloud DNS Provider

This DNS provider allows you to create and manage DNS entries in GCP Cloud DNS.

## Generate Service Account

You need to provide a service account and a key (serviceaccount.json) to allow the dns-controller-manager to authenticate and execute calls to Cloud DNS.

For details on Cloud DNS see https://cloud.google.com/dns/docs/zones, and on Service Accounts see https://cloud.google.com/iam/docs/service-accounts

## Required permissions

The service account needs permissions on the hosted zone to list and change DNS records. For details on which permissions or roles are required see https://cloud.google.com/dns/docs/access-control. A possible role is `roles/dns.admin` "DNS Administrator".

Create a key for the configured service account. GCP will generate a `serviceaccount.json` file as key, similar to the example below. Keep this file safe as it won't be accessible again.

```json
{
  "type": "service_account",
  "project_id": "...",
  "private_key_id": "...",
  "private_key": "-----BEGIN PRIVATE KEY----- ... -----END PRIVATE KEY-----\n",
  "client_email": "...",
  "client_id": "...",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/..."
}
```


## Using the Service Account Key

Create a `Secret` resource with the data field `serviceaccount.json` with the value being the base64 encoded string, e.g. with

```bash
$ encoded_key=`cat serviceaccount.json | base64`
$ echo $encoded_key
```

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: google-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with json key from service account creation (encoded as base64)
  # see https://cloud.google.com/iam/docs/creating-managing-service-accounts
  serviceaccount.json: ...
```