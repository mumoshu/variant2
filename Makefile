.PHONY: build
build:
	go build -o variant ./

.PHONY: test
test: build
	go vet ./...
	PATH=$(PWD):$(PATH) go test ./...

bin/golangci-lint:
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s v1.17.1

.PHONY: lint
lint: bin/golangci-lint
	bin/golangci-lint run
