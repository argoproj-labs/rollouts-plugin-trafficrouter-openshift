#!/bin/bash

set -ex

function cleanup {
    echo "Cleaning up"
    killall openshift || true
    killall main || true
    killall go || true
}

trap cleanup EXIT

# Configure the plugin and run e2e tests
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

if [ $# -eq 0 ]; then
    echo "Provide a path to the Argo Rollouts repository"
    exit 1
fi
ROLLOUTS_DIR=$1


kubectl delete ns argo-rollouts || true

kubectl create ns argo-rollouts
kubectl config set-context --current --namespace=argo-rollouts

# Build the plugin binary
cd $SCRIPT_DIR/..
make build

mv dist/rollouts-plugin-trafficrouter-openshift $ROLLOUTS_DIR/plugin-bin

# Create the rollouts configmap with the plugin path
cat << EOF | kubectl apply -f - 
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
  namespace: argo-rollouts
data:
  trafficRouterPlugins: |-
    - name: "argoproj-labs/openshift"
      location: "file://plugin-bin/rollouts-plugin-trafficrouter-openshift"
EOF

# Start Argo Rollouts in the background
cd $ROLLOUTS_DIR
kubectl apply -k manifests/crds
rm -f /tmp/rollouts-controller.log || true
go run ./cmd/rollouts-controller/main.go 2>&1 | tee /tmp/rollouts-controller.log &

# wait for the controller to start
sleep 30

# Start the e2e tests
cd $SCRIPT_DIR/..
make test-e2e

cleanup