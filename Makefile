.PHONY: build
build:
	go build -o variant ./pkg/cmd

bin/goimports:
	GOBIN=$(PWD)/bin go install golang.org/x/tools/cmd/goimports

.PHONY: fmt
fmt: bin/goimports
	bin/goimports -w pkg .
	gofmt -w -s pkg .

.PHONY: test
test: build
	go vet ./...
	PATH=$(PWD):$(PATH) go test -race -v ./...

bin/golangci-lint:
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s v1.23.1

.PHONY: lint
lint: bin/golangci-lint
	bin/golangci-lint run --tests \
	  --enable-all \
	  --disable gochecknoglobals \
	  --disable gochecknoinits \
	  --disable gomnd,funlen,prealloc,gocritic,lll,gocognit

.PHONY: smoke
smoke: build
	make build
	./variant export go examples/simple build/simple
	go build -o build/simple/simple ./build/simple
	build/simple/simple -h | tee smoke.log
	grep "Namespace to interact with" smoke.log
