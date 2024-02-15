
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
	go test -coverprofile cover.out `go list ./...`
