# POC load-test helper.
#
# Run from this directory:
#   cd scripts/channel-benchmark/py/fy_poc_loadtest
#
# Required for real runs:
#   export FY_API_URL=https://api-test.tracenex.cn
#   export FY_API_USER_TOKEN=sk-...
#
# Optional overrides:
#   make run MODEL=deepseek-r1 CONFIG=../poc-loadtest.yaml
#   make quick-report MODEL=deepseek-r1 CONCURRENCIES=1,10

PY_DIR := ..
CONFIG ?= poc-loadtest.yaml
MODEL ?=
CONCURRENCIES ?=
QUICK_CONCURRENCIES ?= 1
OUTPUT_DIR ?= poc-loadtest-results
FORMATS ?= json,csv,markdown

MODEL_ARG := $(if $(MODEL),--model $(MODEL),)
CONCURRENCY_ARG := $(if $(CONCURRENCIES),--concurrencies $(CONCURRENCIES),)

.PHONY: help install dry-run run quick-report test latest-report list-reports clean

help:
	@echo "fy-poc-loadtest Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  make install        Install Python package in editable mode"
	@echo "  make dry-run        Validate config without sending requests"
	@echo "  make run            Run full POC load test and write reports"
	@echo "  make quick-report   Run C=$(QUICK_CONCURRENCIES) smoke report for template check"
	@echo "  make test           Run fy-poc-loadtest unit tests"
	@echo "  make latest-report  Print latest generated Markdown report path"
	@echo "  make list-reports   List generated reports"
	@echo "  make clean          Remove generated POC reports"
	@echo ""
	@echo "Variables:"
	@echo "  CONFIG=$(CONFIG)"
	@echo "  MODEL=$(MODEL)"
	@echo "  CONCURRENCIES=$(CONCURRENCIES)  (empty means use config file)"
	@echo "  OUTPUT_DIR=$(OUTPUT_DIR)"
	@echo "  FORMATS=$(FORMATS)"

install:
	cd $(PY_DIR) && uv pip install --python .venv/bin/python -e ".[dev]"

dry-run:
	cd $(PY_DIR) && python -m fy_poc_loadtest \
		-c $(CONFIG) \
		$(MODEL_ARG) \
		$(CONCURRENCY_ARG) \
		--output $(OUTPUT_DIR) \
		--formats $(FORMATS) \
		--dry-run

run:
	cd $(PY_DIR) && python -m fy_poc_loadtest \
		-c $(CONFIG) \
		$(MODEL_ARG) \
		$(CONCURRENCY_ARG) \
		--output $(OUTPUT_DIR) \
		--formats $(FORMATS)

quick-report:
	cd $(PY_DIR) && python -m fy_poc_loadtest \
		-c $(CONFIG) \
		$(MODEL_ARG) \
		--concurrencies $(QUICK_CONCURRENCIES) \
		--output $(OUTPUT_DIR) \
		--formats $(FORMATS)

test:
	cd $(PY_DIR) && pytest tests/test_poc_loadtest.py -q

latest-report:
	@ls -t $(OUTPUT_DIR)/poc_loadtest_*.md 2>/dev/null | head -1 || true

list-reports:
	@ls -lh $(OUTPUT_DIR)/poc_loadtest_* 2>/dev/null || true

clean:
	rm -f $(OUTPUT_DIR)/poc_loadtest_*
