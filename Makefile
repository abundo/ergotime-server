.PHONY: all build frontend ensure-dist server release run tidy test clean docker-up docker-down podman-up podman-down

BINARY := bin/ergotime-server
CONFIG ?= config/config.debug.yaml

all: frontend server

# Production single-binary build: Vue assets are go:embed'd into the binary.
release: frontend
	go build -ldflags="-s -w" -o $(BINARY) ./cmd/server

server: ensure-dist
	go build -o $(BINARY) ./cmd/server

# go:embed requires web/dist at compile time. Build frontend if missing.
ensure-dist:
	@if [ ! -f web/dist/index.html ]; then \
		echo "web/dist missing — building frontend for embed..."; \
		$(MAKE) frontend; \
	fi

frontend:
	cd web && npm ci && npm run build

frontend-dev:
	cd web && npm run dev

run: server
	./$(BINARY) -config $(CONFIG)

tidy:
	go mod tidy

test:
	go test ./...

clean:
	rm -rf bin/ web/dist web/node_modules

docker-up:
	docker compose up -d

docker-down:
	docker compose down

podman-up:
	podman compose up -d

podman-down:
	podman compose down
