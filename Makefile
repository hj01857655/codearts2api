.PHONY: all linux windows test docker clean release

GO ?= go
BINDIR := bin

# 版本注入：正式发版走 goreleaser（tag 触发）；本地 make 用 git 描述当前状态，
# 这样 `codearts2api -version` 在任何构建里都有意义。
# 注意：-buildvcs=false 与显式 ldflags 同时存在时，版本仍以上面的变量为准。
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)

all: linux

linux:
	@mkdir -p $(BINDIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api ./cmd/server
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api-login ./cmd/login
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api-credit ./cmd/credit
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api-apply ./cmd/apply
	@echo "linux binaries -> $(BINDIR)/ ($(VERSION))"

windows:
	@mkdir -p $(BINDIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api.exe ./cmd/server
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api-login.exe ./cmd/login
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api-credit.exe ./cmd/credit
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINDIR)/codearts2api-apply.exe ./cmd/apply
	@echo "windows binaries -> $(BINDIR)/ ($(VERSION))"

test:
	$(GO) test ./...

docker:
	docker compose up -d --build

clean:
	rm -rf $(BINDIR) data
