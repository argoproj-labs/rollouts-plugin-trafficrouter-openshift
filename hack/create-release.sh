#!/bin/bash

# This script creates a pre-release release based on the current commit.

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

set -ex

cd "$SCRIPT_DIR/.."
make clean release

cd "$SCRIPT_DIR"


BRANCH=`git rev-parse --abbrev-ref HEAD`
COMMIT=`git rev-parse HEAD`

if [ "$BRANCH" != "main" ]; then
  echo "branch must be main"
  exit 1
fi

gh release create --repo argoproj-labs/rollouts-plugin-trafficrouter-openshift -t "$COMMIT" -p "commit-$COMMIT" ../dist/rollouts-plugin-trafficrouter-openshift-* | tee
