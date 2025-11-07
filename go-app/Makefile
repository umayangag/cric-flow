# Added Mockery targets for generating interface mocks

.PHONY: mocks mocks-clean

# Generate mocks using mockery with project config
mocks:
	@command -v mockery >/dev/null 2>&1 || (echo "Install mockery: go install github.com/vektra/mockery/v2@latest" && exit 1)
	mockery --config .mockery.yaml

# Clean generated mocks
mocks-clean:
	rm -rf internal/**/mocks
