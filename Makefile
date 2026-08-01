.PHONY: generate build test run
generate:
	go run github.com/a-h/templ/cmd/templ generate
build: generate
	go build -o bin/tour ./cmd/tour
test: generate
	go test ./...
run: build
	./bin/tour
