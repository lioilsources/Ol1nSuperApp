BINARY = superol1n
NAS_HOST = nas.local
NAS_PATH = /usr/local/bin/superol1n

.PHONY: build build-nas deploy run tidy

build:
	go build -o $(BINARY) ./cmd/server

build-nas:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY) ./cmd/server

deploy: build-nas
	scp $(BINARY) $(NAS_HOST):$(NAS_PATH)
	ssh $(NAS_HOST) "sudo systemctl restart superol1n"

run: build
	./$(BINARY)

tidy:
	go mod tidy
