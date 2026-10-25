VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BIN     ?= memalyze
GO      ?= go
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt demo run http docker-build docker-up docker-down clean

all: test build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/memalyze

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

demo:
	$(GO) run ./cmd/memalyze demo

# MCP over stdio (what a harness spawns)
run: build
	./$(BIN)

# MCP over Streamable HTTP (local only; set MEMALYZE_HTTP_TOKEN to expose it)
http: build
	MEMALYZE_HTTP_ADDR=$${MEMALYZE_HTTP_ADDR:-127.0.0.1:8080} ./$(BIN)

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t memalyze:$(VERSION) .

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

# 把 dsh/agent skill 打成可上传的包（目录提交用）
.PHONY: skill
skill:
	@mkdir -p dist
	@tar -czf dist/memalyze-skill-$(VERSION).tar.gz -C skills memalyze
	@tar -tzf dist/memalyze-skill-$(VERSION).tar.gz
	@ls -lh dist/memalyze-skill-$(VERSION).tar.gz

clean:
	rm -f $(BIN)
	rm -rf dist

# 交叉编译（S21 发布用；CGO 已禁用，所以没有工具链依赖）
.PHONY: release
release:
	@mkdir -p dist
	@for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${target%/*}; arch=$${target#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/$(BIN)-$$os-$$arch$$ext ./cmd/memalyze; \
	done
	@cd dist && (shasum -a 256 memalyze-* 2>/dev/null || sha256sum memalyze-*) | grep -v '\.mcpb$$' > SHA256SUMS
	@echo "$(VERSION)" > dist/VERSION
	@ls -lh dist

# .mcpb bundles for Claude Desktop / MCPB clients + deploy/registry/server.json.
# Run on a tagged commit: bundles must contain the same code as the tag they claim.
.PHONY: mcpb
mcpb: release
	VERSION=$$(sed 's/^v//' dist/VERSION) ./scripts/build-mcpb.sh
	./scripts/gen-server-json.py --version $$(sed 's/^v//' dist/VERSION)

# Pre-upload guard: fails if deploy/registry/server.json no longer matches dist/.
.PHONY: verify-mcpb
verify-mcpb:
	./scripts/gen-server-json.py --version $$(python3 -c "import json;print(json.load(open('deploy/registry/server.json'))['version'])") --check
