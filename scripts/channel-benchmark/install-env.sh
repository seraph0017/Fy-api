#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PY_DIR="${ROOT_DIR}/py"
VENV_DIR="${PY_DIR}/.venv"
PYTHON_VERSION="3.13"

WITH_DEV=0
WITH_CANARY=0
WITH_IMAGE_CANARY=0
WITH_TIKTOKEN=0
SKIP_FIXTURES=0

usage() {
  cat <<'EOF'
Usage: scripts/channel-benchmark/install-env.sh [options]

Install the local channel-benchmark Python environment.

Options:
  --python VERSION       Python version for uv venv (default: 3.13)
  --with-dev            Install pytest/dev dependencies
  --with-canary         Install text canary MMD dependencies (pulls torch transitively)
  --with-image-canary   Install image canary dependencies (torch/transformers/Pillow)
  --with-tiktoken       Install optional local tokenization fallback
  --all                 Install dev, canary, image-canary, and tiktoken extras
  --skip-fixtures       Do not regenerate committed tiny fixtures
  -h, --help            Show this help

Notes:
  - The script requires uv. Install it first from https://docs.astral.sh/uv/
  - Real benchmark tokens are not installed here. Put reusable base_url/token
    defaults in local YAML files and use environment variables for secrets.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --python)
      PYTHON_VERSION="${2:?--python requires a version}"
      shift 2
      ;;
    --with-dev)
      WITH_DEV=1
      shift
      ;;
    --with-canary)
      WITH_CANARY=1
      shift
      ;;
    --with-image-canary)
      WITH_IMAGE_CANARY=1
      shift
      ;;
    --with-tiktoken)
      WITH_TIKTOKEN=1
      shift
      ;;
    --all)
      WITH_DEV=1
      WITH_CANARY=1
      WITH_IMAGE_CANARY=1
      WITH_TIKTOKEN=1
      shift
      ;;
    --skip-fixtures)
      SKIP_FIXTURES=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    return 1
  fi
}

need_cmd uv || {
  cat >&2 <<'EOF'

uv is required to install the Python benchmark tools.
Install uv first:
  curl -LsSf https://astral.sh/uv/install.sh | sh

EOF
  exit 127
}

echo "creating Python venv: ${VENV_DIR} (python ${PYTHON_VERSION})"
uv venv --python "${PYTHON_VERSION}" "${VENV_DIR}"

extras=(fixtures)
[[ "${WITH_DEV}" -eq 1 ]] && extras+=(dev)
[[ "${WITH_CANARY}" -eq 1 ]] && extras+=(canary)
[[ "${WITH_IMAGE_CANARY}" -eq 1 ]] && extras+=(image-canary)
[[ "${WITH_TIKTOKEN}" -eq 1 ]] && extras+=(tiktoken)

extra_expr="$(IFS=,; echo "${extras[*]}")"
echo "installing fy-channel-qa[${extra_expr}]"
uv pip install --python "${VENV_DIR}/bin/python" -e "${PY_DIR}[${extra_expr}]"

if [[ "${SKIP_FIXTURES}" -eq 0 ]]; then
  echo "regenerating deterministic fixture assets"
  "${VENV_DIR}/bin/python" "${ROOT_DIR}/fixtures/generate_fixtures.py"
fi

cat <<EOF

channel-benchmark environment installed.

Activate it with:
  source ${VENV_DIR}/bin/activate

Available CLIs include:
  fy-smoke, fy-loadtest, fy-quality, fy-canary, fy-conformance, fy-integrity,
  fy-image-loadtest, fy-image-conformance, fy-image-canary, fy-score

Heavy extras are opt-in:
  --with-canary        text canary MMD dependencies
  --with-image-canary  image authenticity dependencies
  --with-tiktoken      token inflation fallback tokenizer
EOF
