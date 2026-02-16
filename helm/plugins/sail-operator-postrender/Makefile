SOURCES := $(wildcard *.go)

build: plugin.wasm

plugin.wasm: $(SOURCES) go.mod go.sum
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .

vet:
	GOOS=wasip1 GOARCH=wasm go vet ./...

test:
	go test -v ./...

clean:
	rm -f plugin.wasm
