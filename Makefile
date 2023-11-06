.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGETARCH) go build -o rollouts-plugin-trafficrouter-openshift ./