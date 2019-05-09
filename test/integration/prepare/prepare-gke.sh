#!/bin/bash

set -ex

SCRIPT_BASEDIR=$(dirname "$0")

export SERVICE_ACCOUNT=integration-test-ext-dns-mgmt@sap-se-gcp-scp-k8s-dev.iam.gserviceaccount.com
SUFFIX=master

usage()
{
    cat <<EOM
Usage:
Creates nodeless Kubernetes cluster on GKE

./prepare-gke.sh [-service-account <account>] [-suffix <suffix>]

Options:
    -service-account <account>     service account to use for creating cluster
    -suffix <suffix>               suffix for clustername

EOM
}

while [ "$1" != "" ]; do
    case $1 in
        -service-account )   shift
                             SERVICE_ACCOUNT=$1
                             shift
                             ;;
        -suffix )            shift
                             SUFFIX=$1
                             shift
                             ;;
        * )                  usage
                             exit 1
    esac
done

if [ "$SERVICE_ACCOUNT" == "" ]; then
  echo Missing service account
  exit 1
fi

if [ "$SUFFIX" == "" ]; then
  echo Missing suffix
  exit 1
fi

CLUSTER_NAME=external-dns-mgmt-test-$SUFFIX

gcloud container clusters create $CLUSTER_NAME \
  --num-nodes 1 --machine-type "g1-small" --image-type "COS" --disk-type "pd-standard" --disk-size "20" --zone "europe-west1-b" \
  --service-account "$SERVICE_ACCOUNT" \
  --no-enable-cloud-logging --no-enable-cloud-monitoring

gcloud container node-pools delete -quiet --cluster=$CLUSTER_NAME default-pool
