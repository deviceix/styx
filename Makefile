.PHONY: all build test clean install uninstall example run-example

GO_BUILD_FLAGS = -v

all: build

build:
	@echo "Building styx..."
	go build $(GO_BUILD_FLAGS) -o bin/styx ./cmd/styx

test:
	@echo "Running tests..."
	go test -v ./...

clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf .styx/
	go clean

install: build
	@echo "Installing styx..."
	cp bin/styx /usr/local/bin/

uninstall:
	@echo "Uninstalling styx..."
	rm -f /usr/local/bin/styx

example: build
	@echo "Building example project..."
	cd examples/simple && ../../bin/styx build

run-example: example
	@echo "Running example..."
	cd examples/simple && ../../bin/styx run