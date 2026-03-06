.PHONY: build run clean vet bootstrap dicts test test-unit test-integration benchmark benchmark-gate lint-metrics

BINARY = vargasparse

build:
	go build -o $(BINARY) ./cmd/vargasparse/

run: build
	./$(BINARY) $(ARGS)

clean:
	rm -f $(BINARY)

vet:
	go vet ./...

bootstrap: dicts
	go mod tidy
	@echo "Checking system dependencies..."
	@which tesseract  > /dev/null 2>&1 || echo "  ✗ tesseract not found — brew install tesseract tesseract-lang"
	@which pdftoppm   > /dev/null 2>&1 || echo "  ✗ pdftoppm not found — brew install poppler"
	@which pdftotext  > /dev/null 2>&1 || echo "  ✗ pdftotext not found — brew install poppler"
	@which ollama     > /dev/null 2>&1 || echo "  · ollama not found — optional for VLM rescue"
	@echo "Bootstrap complete."

dicts:
	@mkdir -p internal/quality/dicts
	@echo "Downloading EN-US dictionary..."
	curl -sL "https://raw.githubusercontent.com/dwyl/english-words/master/words_alpha.txt" \
		-o internal/quality/dicts/en_us.txt
	@echo "Downloading PT-BR dictionary..."
	curl -sL "https://raw.githubusercontent.com/fserb/pt-br/master/lexico" \
		-o internal/quality/dicts/pt_br.txt
	@echo "Done. Word counts:"
	@wc -l internal/quality/dicts/en_us.txt internal/quality/dicts/pt_br.txt

test: test-unit

test-unit:
	go test -v ./...

test-integration:
	go test -v -run Integration ./...

benchmark: build
	./$(BINARY) --engine deterministic --benchmark-report /tmp/vargasparse-benchmark-attention.json test_pdfs/attention.pdf /tmp/attention.txt

benchmark-gate: build
	go run ./cmd/benchmark --manifest test_pdfs/corpus_manifest.json --binary ./$(BINARY) --out-dir /tmp/vargasparse-benchmark

lint-metrics:
	go test -cover ./...
