# Azure DNS Provider for Private Zones

This DNS provider allows you to create and manage DNS entries in private zones of [Azure Private DNS](https://docs.microsoft.com/en-us/azure/dns/private-dns-overview).
For public DNS zones, please see use the provider type [azure-dns](../azure-dns/README.md).

## Create a service principal account

Follow the steps as described in the Azure documentation to [create a service principal account](https://docs.microsoft.com/en-us/azure/dns/dns-sdk#create-a-service-principal-account)
and grant the service principal account 'Private DNS Zone Contributor' permissions to the resource group. 

See also [How to protect private DNS zones and records](https://docs.microsoft.com/en-us/azure/dns/dns-protect-private-zones-recordsets)

## Using the service principal account

Create a `Secret` resource with the data fields `AZURE_SUBSCRIPTION_ID`, `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and `AZURE_CLIENT_SECRET`.
The optional field `AZURE_CLOUD` specifies the Azure Cloud. Allowed values are `AzurePublic` (Azure Public Cloud), `AzureChina`, and `AzureGovernment`.
If not specified, the Azure Public Cloud is assumed.
The values need to be base64 encoded.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: azure-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with values encoded as base64
  # see https://docs.microsoft.com/en-us/azure/dns/dns-sdk#create-a-service-principal-account
  AZURE_SUBSCRIPTION_ID: ...
  AZURE_TENANT_ID: ...
  AZURE_CLIENT_ID: ...
  AZURE_CLIENT_SECRET: ...

  # optional AZURE_CLOUD value specifies the Azure cloud: `AzurePublic`, `AzureChina`, `AzureGovernment` 
  #AZURE_CLOUD: ...

  # Alternatively use Gardener cloud provider credentials convention
  #tenantID: ...
  #subscriptionID: ...
  #clientID: ...
  #clientSecret: ...
``` 
