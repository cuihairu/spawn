GOCMD ?= go
GOFMT ?= gofmt

.PHONY: tidy lint fmt test run

tidy:
	$(GOCMD) mod tidy

fmt:
	$(GOFMT) -w .

lint:
	golangci-lint run ./...

test:
	$(GOCMD) test ./...

run:
	$(GOCMD) run .
