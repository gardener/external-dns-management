# Azure DNS Provider

This DNS provider allows you to create and manage DNS entries in [Azure DNS](https://docs.microsoft.com/en-us/azure/dns/dns-overview). 
For private DNS zones, please see use the provider type [azure-private-dns](../azure-private-dns/README.md).

## Create a service principal account

Follow the steps as described in the Azure documentation to [create a service principal account](https://docs.microsoft.com/en-us/azure/dns/dns-sdk#create-a-service-principal-account)
and grant the service principal account 'DNS Zone Contributor' permissions to the resource group. 

## Using the service principal account

Create a `Secret` resource with the data fields `AZURE_SUBSCRIPTION_ID`, `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and `AZURE_CLIENT_SECRET`.
The optional field `AZURE_CLOUD` specifies the Azure Cloud. Allowed values are `AzurePublic` (Azure Public Cloud), `AzureChina`, and `AzureGovernment`.
If not specified, the public Azure cloud is assumed.
All values need to be base64 encoded.

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
workload identity federation to authenticate to Azure without the need to manage long-lived client secret.
In this case, a `Secret` containing the data fields `token` and `config` is expected.
This secret is not created manually. Instead, Gardener will automatically create and update
it from a `WorkloadIdentity` resource in the project namespace.

Please see the documentation of the Gardener extension `shoot-dns-service` for more [details](https://github.com/gardener/gardener-extension-shoot-dns-service/blob/master/docs/usage/workloadidentity/azure.md).