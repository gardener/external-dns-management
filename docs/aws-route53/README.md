# AWS Route DNS Provider

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
  # Alternatively use Gardener cloud provider credentials convention
  #accessKeyID: ...
  #secretAccessKey: ...
``` 