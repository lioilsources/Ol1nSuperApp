BINARY   = ol1n-vault
NAS_HOST = nas.local
NAS_PATH = /usr/local/bin/ol1n-vault

.PHONY: build build-nas deploy run tidy test vet

build:
	go build -o $(BINARY) ./cmd/vault

build-nas:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY) ./cmd/vault

deploy: build-nas
	scp $(BINARY) $(NAS_HOST):$(NAS_PATH)
	ssh $(NAS_HOST) "sudo systemctl restart ol1n-vault"

run: build
	./$(BINARY)

tidy:
	go mod tidy

test:
	go test ./...

vet:
	go vet ./...
