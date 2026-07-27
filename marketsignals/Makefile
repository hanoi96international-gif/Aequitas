# marketsignals — one command per thing you actually want to do.
#
# The whole path is:  make fetch  →  make analyse  →  make paper
#
# `make fetch` is the only step that needs network access. If you are running
# this where the network is closed, fetch elsewhere and commit the CSVs.

CONFIG ?= config.json
DATA   ?= data
DAYS   ?= 365
MARKET ?= futures
INTERVAL ?= 1h

# Symbols come from the config file, so there is one place to change them.
SYMBOLS = $(shell sed -n 's/.*"symbol"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' $(CONFIG))

.PHONY: help
help:
	@echo "make config    copy the example config into place"
	@echo "make fetch     download bars for every symbol in $(CONFIG)  (needs network)"
	@echo "make analyse   search, build a book, and put both to the hiring panel"
	@echo "make signal    print what the agents think about the latest bar"
	@echo "make paper     run live against a paper account (one instrument)"
	@echo "make book      run the whole universe live as one book"
	@echo "make test      run the test suite"
	@echo "make check     format, vet and test — run before committing"
	@echo ""
	@echo "  CONFIG=$(CONFIG)  DATA=$(DATA)  DAYS=$(DAYS)  MARKET=$(MARKET)"

$(CONFIG):
	cp config.example.json $(CONFIG)
	@echo "wrote $(CONFIG) — edit it, then: make fetch"

.PHONY: config
config: $(CONFIG)

.PHONY: fetch
fetch: $(CONFIG)
	@mkdir -p $(DATA)
	@for s in $(SYMBOLS); do \
		echo "── $$s ──"; \
		go run ./cmd/fetchdata binance -market $(MARKET) -symbol $$s \
			-interval $(INTERVAL) -days $(DAYS) -out $(DATA)/$$s.csv || \
			echo "  skipped $$s"; \
	done
	@echo ""
	@ls -la $(DATA)/*.csv 2>/dev/null | tail -n +1 || echo "nothing downloaded"

.PHONY: analyse
analyse:
	go run ./cmd/signalctl analyse -dir $(DATA) -interval $(INTERVAL)

.PHONY: signal
signal:
	@for f in $(DATA)/*.csv; do \
		echo "── $$f ──"; \
		go run ./cmd/signalctl signal -csv $$f -interval $(INTERVAL) \
			-symbol $$(basename $$f .csv); \
	done

.PHONY: paper
paper:
	go run ./cmd/signald -symbol $(firstword $(SYMBOLS)) -interval $(INTERVAL) \
		-market $(MARKET) -trade -out signals.jsonl

.PHONY: book
book: $(CONFIG)
	go run ./cmd/bookd -config $(CONFIG) -allocator experts -out allocations.jsonl

.PHONY: book-cross
book-cross: $(CONFIG)
	go run ./cmd/bookd -config $(CONFIG) -allocator cross -out allocations.jsonl

.PHONY: experts
experts:
	go run ./cmd/signalctl experts

.PHONY: demo
demo:
	go run ./cmd/signalctl demo

.PHONY: test
test:
	go test ./...

.PHONY: check
check:
	gofmt -l .
	go vet ./...
	go test ./...

.PHONY: clean
clean:
	rm -f signals.jsonl allocations.jsonl
