# AWS Route 53 DNS Provider

This DNS provider allows you to create and manage DNS entries in AWS Route 53. 

## Generate New Access Key

You need to provide an access key (access key ID and secret access key) for AWS to allow the dns-controller-manager to 
authenticate to AWS Route 53.

For details see https://docs.aws.amazon.com/general/latest/gr/managing-aws-access-keys.html

## Required permissions

The user needs permissions on the hosted zone to list and change DNS records. For details on creating an access policy for a user see https://docs.aws.amazon.com/IAM/latest/UserGuide/access_policies_create.html

In this example, the placeholder for the hosted zone is `Z2XXXXXXXXXXXX`

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": "route53:ListResourceRecordSets",
            "Resource": "arn:aws:route53:::hostedzone/*"
        },
        {
            "Sid": "VisualEditor1",
            "Effect": "Allow",
            "Action": "route53:GetHostedZone",
            "Resource": "arn:aws:route53:::hostedzone/Z2XXXXXXXXXXXX"
        },
        {
            "Sid": "VisualEditor2",
            "Effect": "Allow",
            "Action": "route53:ListHostedZones",
            "Resource": "*"
        },
        {
            "Sid": "VisualEditor3",
            "Effect": "Allow",
            "Action": "route53:ChangeResourceRecordSets",
            "Resource": "arn:aws:route53:::hostedzone/Z2XXXXXXXXXXXX"
        }
    ]
}
```

## Using the Access Key

Create a `Secret` resource with the data fields `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
The values are the base64 encoded access key ID and secret access key respectively.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with values encoded as base64
  # see https://docs.aws.amazon.com/general/latest/gr/managing-aws-access-keys.html
  AWS_ACCESS_KEY_ID: ...
  AWS_SECRET_ACCESS_KEY: ...
  # optionally specify the region
  #AWS_REGION: ...
  # optionally specify the token
  #AWS_SESSION_TOKEN: ...
  
  # Alternatively use Gardener cloud provider credentials convention
  #accessKeyID: ...
  #secretAccessKey: ...
``` 

## Using the chain of credential providers

Alternatively the credentials can be provided externally, i.e. by using the
chain of credential providers to search for credentials in environment
variables, shared credential file, and EC2 Instance Roles.

In this case create a `Secret` with the data field `AWS_USE_CREDENTIALS_CHAIN` and set the value to 
`true` (encoded as base64). Typical examples are usage of an AWS Web Identity provider or
[IAM role assigned to the service account](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html).

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: default
type: Opaque
data:
  AWS_USE_CREDENTIALS_CHAIN: dHJ1ZQ==
  # optionally specify the region
  #AWS_REGION: ...
```

You may need to mount an additional volume as the AWS client expects environment variable with token path and volume mount with the token file.
See Helm chart values `custom.volumes` and `custom.volumeMounts`.