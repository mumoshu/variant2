VARIANT_SDK = github.com/mumoshu/variant2/pkg/sdk

# NOTE:
#   You can test the versioned build with e.g. `GITHUB_REF=refs/heads/v0.36.0 make build`
.PHONY: build
build:
	@echo "Building variant"
	@{ \
	set -e ;\
	. hack/sdk-vars.sh ;\
	echo Using $(VAARIANT_SDK).Version=$${VERSION} ;\
	echo Using $(VAARIANT_SDK).ModReplaces=$${MOD_REPLACES} ;\
	set -x ;\
	go build \
	  -ldflags "-X $(VARIANT_SDK).Version=$${VERSION} -X $(VARIANT_SDK).ModReplaces=$${MOD_REPLACES}" \
	  -o variant ./pkg/cmd ;\
	}

bin/goimports:
	echo "Installing goimports"
	@{ \
	set -e ;\
	INSTALL_TMP_DIR=$$(mktemp -d) ;\
	cd $$INSTALL_TMP_DIR ;\
	go mod init tmp ;\
	GOBIN=$(PWD)/bin go install golang.org/x/tools/cmd/goimports ;\
	rm -rf $$INSTALL_TMP_DIR ;\
	}

bin/source-controller:
	echo "Installing source-controller"
	@{ \
	set -e ;\
	INSTALL_TMP_DIR=$$(mktemp -d) ;\
	cd $$INSTALL_TMP_DIR ;\
	go mod init tmp ;\
	GOBIN=$(PWD)/bin go install github.com/fluxcd/source-controller ;\
	rm -rf $$INSTALL_TMP_DIR ;\
	}

bin/gofumpt:
	echo "Installing gofumpt"
	@{ \
	set -e ;\
	INSTALL_TMP_DIR=$$(mktemp -d) ;\
	cd $$INSTALL_TMP_DIR ;\
	go mod init tmp ;\
	GOBIN=$(PWD)/bin go install mvdan.cc/gofumpt ;\
	rm -rf $$INSTALL_TMP_DIR ;\
	}

bin/gci:
	echo "Installing gci"
	@{ \
	set -e ;\
	INSTALL_TMP_DIR=$$(mktemp -d) ;\
	cd $$INSTALL_TMP_DIR ;\
	go mod init tmp ;\
	GOBIN=$(PWD)/bin go install github.com/daixiang0/gci ;\
	rm -rf $$INSTALL_TMP_DIR ;\
	}

.PHONY: source-controller-crds
source-controller-crds:
	# See https://stackoverflow.com/questions/600079/git-how-do-i-clone-a-subdirectory-only-of-a-git-repository/52269934#52269934
	echo "Fetching source-controller crds"
	@{ \
	set -xe ;\
	INSTALL_TMP_DIR=$$(mktemp -d) ;\
	cd $$INSTALL_TMP_DIR ;\
	git clone \
    	  --depth 1 \
    	  --filter=blob:none \
    	  --no-checkout \
    	  https://github.com/fluxcd/source-controller ;\
	cd source-controller ;\
	git checkout main -- config/crd/bases ;\
	rm -rf .git ;\
	cp config/crd/bases/* $(PWD)/examples/advanced/source/crds ;\
	rm -rf $$INSTALL_TMP_DIR ;\
	}

.PHONY: fmt
fmt: bin/goimports bin/gci bin/gofumpt
	gofmt -w -s pkg .
	bin/gofumpt -w . || :
	bin/gci -w -local github.com/mumoshu/variant2 . || :


.PHONY: test
test: build
	go vet ./...
	PATH=$(PWD):$(PATH) go test -race -v ./...

bin/golangci-lint:
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s v1.33.0

.PHONY: lint
lint: bin/golangci-lint
	bin/golangci-lint run --tests ./... \
	  --timeout 5m \
	  --enable-all \
	  --disable gochecknoglobals \
	  --disable gochecknoinits \
	  --disable gomnd,funlen,prealloc,gocritic,lll,gocognit \
	  --disable testpackage,goerr113,exhaustivestruct,wrapcheck,paralleltest

.PHONY: smoke
smoke: export GOBIN=$(shell pwd)/tools
smoke: build
	go get github.com/rakyll/statik

	make build
	rm -rf build/simple
	VARIANT_BUILD_VARIANT_REPLACE=$(shell pwd) \
	  PATH=${PATH}:$(GOBIN) \
	  ./variant export go examples/simple build/simple
	cd build/simple; go build -o simple ./
	build/simple/simple -h | tee smoke.log
	grep "Namespace to interact with" smoke.log

	rm build/simple/simple
	VARIANT_BUILD_VARIANT_REPLACE=$(shell pwd) \
	  PATH=${PATH}:$(GOBIN) \
	  ./variant export binary examples/simple build/simple
	build/simple/simple -h | tee smoke2.log
	grep "Namespace to interact with" smoke2.log

	rm -rf build/import-multi
	VARIANT_BUILD_VER=v0.0.0 \
	  VARIANT_BUILD_VARIANT_REPLACE=$(shell pwd) \
	  PATH=${PATH}:$(GOBIN) \
	  ./variant export binary examples/advanced/import-multi build/import-multi
	build/import-multi foo baz HELLO > build/import-multi.log
	bash -c 'diff <(echo HELLO) <(cat build/import-multi.log)'

	rm build/import-multi.log
	cd build && \
	  ./import-multi foo baz HELLO > import-multi.log && \
	  bash -c 'diff <(echo HELLO) <(cat import-multi.log)'
	# Remote imports are cached and embedded into the binary so it shouldn't be fetched/persisted at run time
	[ ! -e build/.variant2/cache ] || bash -c 'echo build/.variant2/cache check failed; exit 1'
