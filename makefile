.PHONY: run
run:
	go run cmd/main.go

.PHONY: build
build: build-linux

LDFLAGS="-X main.version=$(VERSION)"
DIST_PATH=dist
build-linux:
	rm -rf $(DIST_PATH)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPROXY=https://proxy.golang.org \
		go build -a -installsuffix cgo \
		-ldflags $(LDFLAGS) \
		-o $(DIST_PATH)/app cmd/main.go
