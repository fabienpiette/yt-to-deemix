-include .env
export

BINARY := ytToDeemix
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build run test test-coverage fmt clean build-all
.PHONY: docker-build docker-push docker-release
.PHONY: up down logs
.PHONY: version bump-patch bump-minor bump-major

# Development
build:
	go build $(LDFLAGS) -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test -v -race -count=1 ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@rm -f coverage.out

fmt:
	go fmt ./...
	@test -z "$$(gofmt -l .)" || (echo "files not formatted:" && gofmt -l . && exit 1)

clean:
	rm -f $(BINARY) coverage.out
	rm -rf dist/

# Cross-compilation
build-all: clean
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY)-$$os-$$arch$$ext . ; \
	done

# Docker
docker-build:
	docker build -t $(BINARY):$(VERSION) -t $(BINARY):latest .

docker-push: docker-build
	docker push $(BINARY):$(VERSION)
	docker push $(BINARY):latest

docker-release: docker-build
	docker tag $(BINARY):$(VERSION) ghcr.io/gndm/$(BINARY):$(VERSION)
	docker tag $(BINARY):latest ghcr.io/gndm/$(BINARY):latest
	docker push ghcr.io/gndm/$(BINARY):$(VERSION)
	docker push ghcr.io/gndm/$(BINARY):latest

# Docker Compose
up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

# Versioning
version:
	@echo $(VERSION)

bump-patch:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$latest | sed 's/v//' | cut -d. -f2); \
	patch=$$(echo $$latest | sed 's/v//' | cut -d. -f3); \
	new="v$$major.$$minor.$$((patch+1))"; \
	echo "$$latest -> $$new"; \
	git tag $$new

bump-minor:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$latest | sed 's/v//' | cut -d. -f2); \
	new="v$$major.$$((minor+1)).0"; \
	echo "$$latest -> $$new"; \
	git tag $$new

bump-major:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	new="v$$((major+1)).0.0"; \
	echo "$$latest -> $$new"; \
	git tag $$new
