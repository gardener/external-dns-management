#!/bin/bash

set -e

SCRIPT_BASEDIR=$(dirname "$0")
ROOTDIR=$(realpath "$SCRIPT_BASEDIR/../..")

usage()
{
    echo "Runs integration tests for external-dns-management with mock provider"
    echo "and local Kubernetes cluster in Docker (kind)"
    echo "./run.sh [-r] [--restart] [-v] [--verbose] [-k] [--keep]"
}

while [ "$1" != "" ]; do
    case $1 in
        -r | --restart )        shift
                           NOBOOTSTRAP=true
                           ;;
        -v | --verbose )        shift
                           VERBOSE=true
                           ;;
        -k | --keep )           shift
                           KEEP_CLUSTER=true
                           ;;
        * )                usage
                           exit 1
    esac
done

docker version > /dev/null || (echo "Local Docker installation needed" && exit 1)

if [ "$VERBOSE" != "" ]; then
  VERBOSE_ARG="-v"
  set -x
fi

if [ "$NOBOOTSTRAP" == "" ]; then
  # prepare Kubernetes IN Docker - local clusters for testing Kubernetes
  go get sigs.k8s.io/kind

  # delete old cluster
  kind delete cluster --name integration || true

  # create K8n cluster in docker
  kind create cluster --name integration 
fi

# store tmp kubeconfig
export KUBECONFIG=$(kind get kubeconfig-path --name="integration")
kubectl cluster-info

# run test suite
go test $ROOTDIR/test/integration -count=1 -failfast $VERBOSE_ARG

# cleanup
if [ "$KEEP_CLUSTER" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi

