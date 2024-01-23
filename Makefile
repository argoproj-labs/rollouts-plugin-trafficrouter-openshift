.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGETARCH) go build -o rollouts-plugin-trafficrouter-openshift ./

.PHONY: release
release:
	make BIN_NAME=gateway-api-plugin-darwin-amd64 GOOS=darwin gateway-api-plugin-build
	make BIN_NAME=gateway-api-plugin-darwin-arm64 GOOS=darwin GOARCH=arm64 gateway-api-plugin-build
	make BIN_NAME=gateway-api-plugin-linux-amd64 GOOS=linux gateway-api-plugin-build
	make BIN_NAME=gateway-api-plugin-linux-arm64 GOOS=linux GOARCH=arm64 gateway-api-plugin-build
	make BIN_NAME=gateway-api-plugin-windows-amd64.exe GOOS=windows gateway-api-plugin-build

.PHONY: test
test: ## Run tests.
	go test -coverprofile cover.out `go list ./...`
