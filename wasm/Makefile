.PHONY: build clean tidy

build:
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .

clean:
	rm -f plugin.wasm plugin.wasm.tar.gz

tidy:
	go mod tidy
