
CURRENT_DIR=$(shell pwd)
DIST_DIR=${CURRENT_DIR}/dist


.PHONY: clean
clean:
	rm ${DIST_DIR}/*

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -v -o ${DIST_DIR}/${BIN_NAME} .

# .PHONY: gateway-api-plugin-build
# gateway-api-plugin-build:
# 	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -v -o ${DIST_DIR}/${BIN_NAME} .

.PHONY: release
release:
	make BIN_NAME=rollouts-plugin-trafficrouter-openshift-darwin-amd64 GOOS=darwin build
	make BIN_NAME=rollouts-plugin-trafficrouter-openshift-darwin-arm64 GOOS=darwin GOARCH=arm64 build
	make BIN_NAME=rollouts-plugin-trafficrouter-openshift-linux-amd64 GOOS=linux build
	make BIN_NAME=rollouts-plugin-trafficrouter-openshift-linux-arm64 GOOS=linux GOARCH=arm64 build
	make BIN_NAME=rollouts-plugin-trafficrouter-openshift-windows-amd64.exe GOOS=windows build

.PHONY: test
test: ## Run tests.
	go test -coverprofile cover.out `go list ./... | grep -v 'tests/e2e'`

.PHONY: test-e2e
test-e2e: ## Run e2e tests.
	go test -v -p=1 -timeout=20m -race -count=1 -coverprofile=coverage.out ./tests/e2e

.PHONY: gosec
gosec: go_sec
	$(GO_SEC) -exclude-dir=hack ./...

GO_SEC = $(shell pwd)/bin/gosec
go_sec: ## Download gosec locally if necessary.
	$(call go-get-tool,$(GO_SEC),github.com/securego/gosec/v2/cmd/gosec@latest)

# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

