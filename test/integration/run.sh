#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e
SCRIPT_BASEDIR=$(dirname "$0")
ROOTDIR=$SCRIPT_BASEDIR/../..
echo ROOTDIR: $ROOTDIR
INTEGRATION_KUBECONFIG=$(readlink -f $ROOTDIR/.kubeconfig-kind-integration)

usage()
{
    cat <<EOM
Usage:
Runs integration tests for external-dns-management with local (mock) provider
and local Kubernetes cluster in Docker (kind) or with local kube-apiserver and etcd

./run.sh [-r|--reuse] [-l] [-v] [-k|--keep] [-- <options> <for> <ginkgo>]

Options:
    -r | --reuse     reuse existing kind cluster
    -k | --keep      keep kind cluster after run for reuse or inspection
    -l               use local kube-apiserver and etcd (i.e. no kind cluster)
    -v               verbose output of script (not test itself)

For options of ginkgo run:
    ginkgo -h

Example: ./run.sh -r -k -- -v -focus=Secret -dryRun
EOM
}

while [ "$1" != "" ]; do
    case $1 in
        -r | --restart )   shift
                           NOBOOTSTRAP=true
                           ;;
        -v )               shift
                           VERBOSE=true
                           ;;
        -l )               shift
                           LOCAL_APISERVER=true
                           ;;
        -k | --keep )      shift
                           KEEP_CLUSTER=true
                           ;;
        -- )               shift
                           break
                           ;;
        * )                usage
                           exit 1
    esac
done

if [ "$LOCAL_APISERVER" == "" ]; then
  docker version > /dev/null || (echo "Local Docker installation needed" && exit 1)
fi

if [ "$VERBOSE" != "" ]; then
  set -x
fi

if [ "$NOBOOTSTRAP" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  echo Starting Kubernetes IN Docker...

  # prepare Kubernetes IN Docker - local clusters for testing Kubernetes
  go install sigs.k8s.io/kind

  rm $INTEGRATION_KUBECONFIG || true
  touch $INTEGRATION_KUBECONFIG
  export KUBECONFIG=$INTEGRATION_KUBECONFIG

  # delete old cluster
  kind delete cluster --name integration || true

  # create K8n cluster in docker
  kind create cluster --name integration
fi

cd $ROOTDIR/test/integration

if [ "$LOCAL_APISERVER" != "" ]; then
  unset USE_EXISTING_CLUSTER
  echo using controller runtime envtest

  export ENVTEST_K8S_VERSION=${ENVTEST_K8S_VERSION:-"1.33"}

  echo "> Installing envtest tools@${ENVTEST_K8S_VERSION} with setup-envtest if necessary"
  if ! command -v setup-envtest &> /dev/null ; then
    >&2 echo "setup-envtest not available"
    exit 1
  fi

  # --use-env allows overwriting the envtest tools path via the KUBEBUILDER_ASSETS env var
  export KUBEBUILDER_ASSETS="$(setup-envtest use --use-env -p path ${ENVTEST_K8S_VERSION})"
  echo "using envtest tools installed at '$KUBEBUILDER_ASSETS'"
else
  export USE_EXISTING_CLUSTER=true
  export KUBECONFIG=$INTEGRATION_KUBECONFIG
  kubectl cluster-info
fi

# run test suite
GINKGO=${GINKGO:-ginkgo}
${GINKGO} -fail-fast -trace "$@"
RETCODE=$?

cd -

# cleanup
if [ "$KEEP_CLUSTER" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi

exit $RETCODE