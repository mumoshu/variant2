.PHONY: build
build:
	go build -o variant ./

.PHONY: test
test: build
	go vet ./...
	PATH=$(PWD):$(PATH) go test ./...
