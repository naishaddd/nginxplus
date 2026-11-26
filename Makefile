.PHONY: build test vet lint assets run demo demo-down docker

build:            ## Build the binary into ./bin/nginxpp
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/nginxpp ./cmd/nginxpp

test:             ## Run unit tests (no Docker needed)
	go test -race ./...

vet:
	go vet ./...

lint:             ## Requires golangci-lint
	golangci-lint run

assets:           ## Rebuild Tailwind CSS + vendored fonts/icons/xterm (needs Node 18+)
	./scripts/build-assets.sh

run: build        ## Run locally against TARGET_CONTAINER (needs /var/run/docker.sock)
	NGINXPP_SETUP_CODE=dev NGINXPP_DATA_DIR=./.data ./bin/nginxpp

demo:             ## Start a demo nginx + NGINX++ and print the first-run setup code
	docker compose -f examples/demo/docker-compose.yml up -d --build
	@./scripts/show-setup-code.sh nginxpp-demo

demo-down:
	docker compose -f examples/demo/docker-compose.yml down -v

docker:
	docker build -t nginxpp:local .
