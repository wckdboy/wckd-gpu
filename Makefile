BIN := bin/wckd

.PHONY: build test check

build:
	mkdir -p bin
	cd cli && CGO_ENABLED=0 go build -trimpath -o ../$(BIN) ./cmd/wckd

test:
	cd cli && go test ./...

check: build test
	./$(BIN) --help >/dev/null
	./$(BIN) config check --help >/dev/null
	./$(BIN) offers --help >/dev/null
	./$(BIN) start --help >/dev/null
	./$(BIN) status --help >/dev/null
	./$(BIN) stop --help >/dev/null
	./$(BIN) sweeper --help >/dev/null
	bash -n sidecar/lib.sh sidecar/bootstrap.sh sidecar/drain.sh
