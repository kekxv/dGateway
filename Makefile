# Makefile for dGateway

# Default target
.PHONY: help
help:
	@echo "dGateway Makefile"
	@echo "Usage:"
	@echo "  make build         - Build the dGateway binary"
	@echo "  make certs         - Generate CA and server certificates"
	@echo "  make run-http      - Run dGateway with HTTP on port 8080"
	@echo "  make run-https     - Run dGateway with HTTPS support on port 8443"
	@echo "  make clean         - Remove generated files"

# Build the dGateway binary
.PHONY: build
build:
	go build -o dgateway .

# Generate certificates
.PHONY: certs
certs: build
	./dgateway -gen-certs

# Run with HTTP only
.PHONY: run-http
run-http: build
	./dgateway -port=8080 -target="http://httpbin.org" -db="requests.db"

# Run with HTTPS support
.PHONY: run-https
run-https: build certs
	./dgateway -port=8443 -target="https://httpbin.org" -db="requests.db" -enable-https

# Clean generated files
.PHONY: clean
clean:
	rm -f dgateway
	rm -f requests.db
	rm -f certs/*.crt certs/*.key