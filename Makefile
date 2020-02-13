export GO111MODULE=on

.PHONY: build
build:
    go get -v -t ./...
	go generate
	go build -v .
	GOOS=windows GOARCH=amd64 go build -v .

.PHONY: test
test: ## go test
	go test -v -cover .

.PHONY: clean
clean: ## go clean
	rm -rf ./statik
	go clean -cache -testcache

.PHONY: analyze
analyze: ## do static code analysis
	goimports -l -w .
	go vet ./...
	golint ./...

.PHONY: all
all: test build
