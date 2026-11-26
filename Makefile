.PHONY: build test lint clean

build:
	go build -o reeltube ./cmd/reeltube

test:
	go test -v ./...

lint:
	go vet ./...

clean:
	rm -f reeltube
