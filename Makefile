SPEC_BROKERS := kis kiwoom ls toss

.PHONY: build run test clean deps dev-tools lint fmt vet mock \
	spec-fetch-all spec-generate-all spec-refresh-all spec-check-all \
	$(foreach b,$(SPEC_BROKERS),$(b)-spec-fetch $(b)-spec-generate $(b)-spec-refresh $(b)-spec-check $(b)-spec-all)

# Build the application
build:
	go build -o bin/krsec ./cmd/krsec

# Run the application
run:
	go run ./cmd/krsec -config config.yaml

# Run tests
test:
	go test -v ./...

# Generate mocks
mock:
	go run github.com/vektra/mockery/v3@v3.6.4 --config .mockery.yml

# Clean build artifacts
clean:
	rm -rf bin/

# Download dependencies
deps:
	go mod download
	go mod tidy

# Install development tools
dev-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# Lint code
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# --- Documented spec workflow (per broker) ---------------------------------
# Each broker follows the same layout:
#   snapshot:  pkg/<broker>/specs/documented_endpoints.json
#   spec out:  pkg/<broker>/specs/documented_specs_generated.go
#   types out: pkg/<broker>/specs/documented_endpoint_types_generated.go (KIS/Kiwoom only)
# Targets: <broker>-spec-{fetch,generate,refresh,check,all}, e.g. `make kis-spec-refresh`.

spec_types_out_kis    = --types-out pkg/kis/specs/documented_endpoint_types_generated.go
spec_types_out_kiwoom = --types-out pkg/kiwoom/specs/documented_endpoint_types_generated.go

# spec_rules(broker): expands to fetch/generate/refresh/check/all targets.
define spec_rules
$(1)-spec-fetch:
	go run ./cmd/$(1)-specgen fetch --out pkg/$(1)/specs/documented_endpoints.json

$(1)-spec-generate:
	go run ./cmd/$(1)-specgen generate --in pkg/$(1)/specs/documented_endpoints.json --spec-out pkg/$(1)/specs/documented_specs_generated.go $(spec_types_out_$(1))

$(1)-spec-refresh:
	go run ./cmd/$(1)-specgen refresh --snapshot pkg/$(1)/specs/documented_endpoints.json --spec-out pkg/$(1)/specs/documented_specs_generated.go $(spec_types_out_$(1))

$(1)-spec-check:
	go run ./cmd/$(1)-specgen check --in pkg/$(1)/specs/documented_endpoints.json --spec-out pkg/$(1)/specs/documented_specs_generated.go $(spec_types_out_$(1))

$(1)-spec-all: $(1)-spec-fetch $(1)-spec-generate $(1)-spec-refresh $(1)-spec-check
endef

$(foreach b,$(SPEC_BROKERS),$(eval $(call spec_rules,$(b))))

# Aggregates across every broker
spec-fetch-all: $(addsuffix -spec-fetch,$(SPEC_BROKERS))
spec-generate-all: $(addsuffix -spec-generate,$(SPEC_BROKERS))
spec-refresh-all: $(addsuffix -spec-refresh,$(SPEC_BROKERS))
spec-check-all: $(addsuffix -spec-check,$(SPEC_BROKERS))
