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

## Using Workload Identity - Trust Based Authentication

In the context of [[GEP-26] Workload Identity - Trust Based Authentication](https://github.com/gardener/gardener/issues/9586),
when the dns-controller-manager is deployed on a Gardener seed, you can also use
workload identity federation to authenticate to Azure without the need to manage long-lived access keys.
In this case, a `Secret` containing the data fields `token` and `config` is expected.
This secret is not created manually. Instead, Gardener will automatically create and update
it from a `WorkloadIdentity` resource in the project namespace.

Please see the documentation of the Gardener extension `shoot-dns-service` for more details.