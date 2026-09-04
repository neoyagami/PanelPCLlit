.PHONY: build build-qt appimage screenshots test run run-qt

build:
	go build -trimpath -ldflags="-s -w" -o build/panelpc ./cmd/panelpc

build-qt:
	go build -tags qt -trimpath -ldflags="-s -w" -o build/panelpc-qt ./cmd/panelpc-qt

appimage:
	./packaging/build-appimage.sh

screenshots: build-qt
	@panelpc_config_dir=$$(mktemp -d); \
	trap 'rm -rf "$$panelpc_config_dir"' EXIT; \
	QT_QPA_PLATFORM=offscreen XDG_CONFIG_HOME="$$panelpc_config_dir" ./build/panelpc-qt --screenshots assets

test:
	go test ./...

run:
	go run ./cmd/panelpc

run-qt:
	go run -tags qt ./cmd/panelpc-qt
