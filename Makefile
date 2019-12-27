.PHONY: build
build:
	go build -o variant2 ./

.PHONY: test
test: build
	go test ./...
