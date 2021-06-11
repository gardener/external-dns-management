# OpenStack Designate DNS Provider

This DNS provider allows you to create and manage DNS entries in [OpenStack Designate](https://docs.openstack.org/designate/latest/). 

## Credentials

The authentication uses keystone with username and password.

See [Keystone username/password](https://docs.openstack.org/keystone/latest/user/supported_clients.html)

At least `OS_USERNAME`, `OS_PASSWORD`, `OS_AUTH_URL`, and `OS_PROJECT_NAME` (or `OS_PROJECT_ID`) have to be
provided.

These are the supported keys:

- `OS_AUTH_URL` - Identity endpoint URL.
- `OS_PASSWORD` - Password.
- `OS_PROJECT_NAME` - Project name.
- `OS_PROJECT_ID` - Project id.
- `OS_REGION_NAME` - Region name, optional.
- `OS_USERNAME` - Username.
- `OS_TENANT_NAME` - Tenant name (deprecated see `OS_PROJECT_NAME` and `OS_PROJECT_ID`).
- `OS_DOMAIN_NAME` - Name of the domain.
- `OS_DOMAIN_ID` - Id of the domain.
- `OS_USER_DOMAIN_NAME` - Name of the user’s domain.
- `OS_USER_DOMAIN_ID` - Id of the user’s domain.

For more details see [AuthInfo type](https://pkg.go.dev/github.com/gophercloud/utils/openstack/clientconfig#AuthInfo)

## Required permissions

`dns_viewer` and `dns_webmaster` roles are needed.

## Using the Access Key

Create a `Secret` resource with the complete set of keys .
All values are base64 encoded.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: openstack-credentials
  namespace: default
type: Opaque
data:
  # Replace '...' with values encoded as base64.
  # For details about key name
  # see https://docs.openstack.org/python-openstackclient/pike/cli/man/openstack.html#environment-variables
  OS_AUTH_URL: ...
  #OS_REGION_NAME: ... (optional)
  OS_DOMAIN_NAME: ...
  # OS_DOMAIN_ID: ... (either name or ID has to be provided)
  OS_PROJECT_NAME: ...
  # OS_PROJECT_ID: ... (either name or ID has to be provided)
  OS_USERNAME: ...
  OS_PASSWORD: ...
  # CACERT: ... (optional)
  # CLIENTCERT: (optional)
  # CLIENTKEY: (required for CLIENTCERT)
  # INSECURE: (optional) true/false
  
  # Alternatively use Gardener cloud provider credentials convention
  #OS_AUTH_URL: ... (always needed)
  #OS_REGION_NAME: ... (optional)

  # Alternatively user domain name and id can be provided via
  #OS_USER_DOMAIN_NAME: ...
  #OS_USER_DOMAIN_ID: ...
  #domainName: ...
  #domainID: ...
  #tenantName: ...
  #tenantID: ...
  #username: ...
  #password: ...
  #userDomainID: ... (optional)
  #userDomainName: ... (optional)
``` 
