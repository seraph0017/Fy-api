# Plan: Unified Channel Benchmark Runner

**Generated**: 2026-07-05  
**Estimated Complexity**: Medium-High

## Overview

Build a script-first benchmark orchestrator so channel tests no longer depend on an AI agent to decide the execution sequence. The new CLI should accept only frequently changing values through flags, read reusable defaults and secrets from YAML, run models one by one by default, emit per-model reports, record module-level timing, and produce a unified A/B/C/D/F scorecard with plus/minus grades for text, image, and video models.

The orchestrator should reuse the existing tools instead of replacing them:

- `fy-loadtest` for text load and latency.
- `fy-quality` for deterministic and optional judge-based quality.
- `fy-canary` for text authenticity when a trusted baseline is configured.
- `fy-conformance` and `fy-integrity` for protocol and integrity checks.
- `fy-image-loadtest`, `fy-image-conformance`, and `fy-image-canary` for image models.
- `fy-eval` video runners initially for video smoke/load.
- `fy-score` for final aggregation, extended where needed for video and plus/minus grading.

### Smoke Runner Position

`fy-smoke` is now the canonical smoke and Prometheus exporter entrypoint.
It covers the old smoke runner's functional surface in Python: admin channel
lookup, channel pinning, stream/non-stream chat probes, TTFT/ITL/E2E/usage
metrics, JSON/CSV exports, long-thinking preset, and `/metrics` exposition.
The unified `fy-benchmark quick` profile should call `fy-smoke` rather than
maintaining a separate smoke implementation.

Practical migration target:

```bash
fy-benchmark --config benchmark.local.yaml --channel-id 42 --model gpt-4o-mini --type text --mode quick
fy-smoke --config smoke.local.yaml --prom-listen :9090 --prom-interval 5m
```

## Target Workflow

Example command:

```bash
cd scripts/channel-benchmark/py
fy-benchmark \
  --config benchmark.local.yaml \
  --channel-id 42 \
  --model gpt-image-2 \
  --model gpt-image-2-mini \
  --type image \
  --mode standard
```

Default execution:

1. Resolve reusable config from YAML.
2. Apply CLI flags for target channel and model IDs.
3. Create a run directory such as `benchmark-results/ch42/20260705-153000/`.
4. For each model, run configured modules strictly serially.
5. Write each module's raw JSON/CSV/Markdown into `models/<model>/`.
6. Append module timing events to `run.timeline.jsonl`.
7. Generate per-model scorecard and Markdown report.
8. Generate a channel-level summary across all models.

## CLI And YAML Contract

### CLI Flags

Use CLI flags for values that usually change per run:

- `--config PATH`: defaults to `benchmark.local.yaml`.
- `--channel-id INT`: required unless config provides a default target.
- `--channel-name TEXT`: optional display-only label. Execution should use `channel_id` as the source of truth. If omitted, reports should display `channel-<id>` or auto-fill the name from admin metadata when available.
- `--model MODEL`: repeatable; each model runs separately.
- `--models MODEL1,MODEL2`: optional comma-separated alternative.
- `--type text|image|video|auto`: defaults to `auto`; explicit type preferred.
- `--protocol PROTOCOL`: repeatable optional protocol selector. Defaults are derived from `--type` and model capability. Supports comma-separated aliases such as `chat,responses,claude_messages`.
- `--protocol-mode default|required|all`: controls protocol selection strictness. `default` tests the canonical protocol for the model type; `required` tests only explicitly requested protocols; `all` tests all known compatible protocols for that type.
- `--region cn|hk|auto`: target environment selection. `auto` should infer from model/provider metadata when possible.
- `--mode quick|standard|strict|deep`: required or default from YAML.
- `--parallel-models N`: defaults to `1`; strict serial by default.
- `--baseline-channel-id INT`: optional trusted comparison channel for canary.
- `--baseline-channel-name TEXT`: optional display name for baseline reports.
- `--with-judge`: enables LLM judge rubric grading.
- `--with-embedding`: enables embedding similarity and drift probes.
- `--with-canary`: enables baseline/audit canary probes.
- `--skip-load`, `--skip-quality`, `--skip-canary`, `--skip-integrity`, `--skip-conformance`: operational overrides.
- `--output-dir PATH`: override result root.

### YAML Defaults

Use YAML for reusable values, secrets, and run profiles:

```yaml
# benchmark.local.yaml
# Local-only file. Do not commit real tokens.

gateway:
  base_url: "https://api-test.tracenex.cn"

tokens:
  # User API key used to send real benchmark traffic through Fy-api.
  # Obtain from TraceNex user console/API key page. For channel pinning,
  # this token must belong to an admin user, otherwise Fy-api rejects forced
  # channel selection.
  user_token: "${FY_API_USER_TOKEN}"

  # Optional judge token used only when --with-judge is enabled.
  # This may point to the same gateway or another judge-capable endpoint.
  judge_token: "${FY_API_JUDGE_TOKEN:-}"

  # Optional embedding token used only when --with-embedding or canary drift
  # is enabled. The configured embedding model must be available on the
  # selected endpoint.
  embedding_token: "${FY_API_EMBEDDING_TOKEN:-}"

judge:
  base_url: "https://api-test.tracenex.cn"
  model: "claude-haiku-4-5-20251001"

embedding:
  base_url: "https://api-test.tracenex.cn"
  model: "text-embedding-v1"

defaults:
  mode: "standard"
  region: "auto"
  output_dir: "benchmark-results"
  parallel_models: 1
  output_formats: ["json", "csv", "markdown"]

regions:
  cn:
    base_url: "https://api-test.tracenex.cn"
    description: "Use for domestic China providers and CN customer-facing channels."
  hk:
    base_url: "https://api-test.aitracenex.com"
    description: "Use for overseas providers and HK/international customer-facing channels."

model_routing:
  # Optional registry for automatic region/type/protocol defaults.
  # CLI flags always override this mapping.
  domestic_keywords: ["qwen", "doubao", "deepseek", "kimi", "zhipu", "baidu", "hunyuan", "minimax", "step", "yi", "wan", "seedance", "kling", "jimeng"]
  overseas_keywords: ["gpt", "claude", "gemini", "sora", "veo", "dall-e", "imagen", "mistral", "grok", "xai"]

profiles:
  quick:
    text: ["smoke", "load_light", "conformance", "integrity_stateless"]
    image: ["smoke", "image_load_light", "image_conformance_core"]
    video: ["video_smoke", "video_submit_load_light"]
  standard:
    text: ["smoke", "load", "quality_deterministic", "conformance", "integrity"]
    image: ["smoke", "image_load", "image_conformance", "image_quality_no_judge", "image_safety"]
    video: ["video_smoke", "video_submit_load"]
  strict:
    text: ["smoke", "load_strict", "quality_deterministic_extended", "conformance", "integrity", "canary_if_baseline"]
    image: ["smoke", "image_load_strict", "image_conformance", "image_quality_no_judge_extended", "image_safety", "image_canary_if_baseline"]
    video: ["video_smoke", "video_submit_load_strict", "video_fetch_completion"]
  deep:
    text: ["smoke", "load", "quality_full", "canary", "conformance", "integrity"]
    image: ["smoke", "image_load", "image_conformance", "image_canary", "image_safety"]
    video: ["video_smoke", "video_submit_load", "video_fetch_completion"]
```

## Run Modes

### Quick

Fast triage mode. Use this to answer "is the channel alive and obviously broken?"

- Shorter load runs.
- Minimal quality checks.
- No judge, embedding, or canary unless explicitly requested.
- Best for quick incident diagnosis.

### Standard

Default recommendation for normal channel checks.

- Runs enough performance, protocol, deterministic quality, and integrity coverage to catch common regressions.
- Keeps expensive judge/embedding/canary disabled by default.
- Produces a balanced grade without trying to exhaustively find every weakness.

### Strict

High-standard audit mode. Use this when the goal is to surface channel weaknesses before promotion, customer POC, or supplier comparison.

- Uses stricter pass thresholds than `standard`.
- Runs longer or heavier load profiles.
- Expands deterministic quality coverage where datasets are available.
- Runs canary automatically if `--baseline-channel-id` or vendor baseline config is present.
- Does not require LLM judge by default, but lowers tolerance for deterministic, protocol, latency, safety, and integrity failures.
- Report should lead with a "Weaknesses / 风险点" section before the score summary.

Strict mode should be problem-oriented: the output should explain what is weak, what likely caused it, and whether it is blocking or advisory.

### Deep

Full expensive validation mode.

- Enables judge, embedding, canary, and heavier image authenticity checks when credentials/config are present.
- Best for final supplier verification or post-remediation validation.
- More complete than strict, but not necessarily harsher; strict is the "high bar" mode, deep is the "more probes" mode.

## Protocol Compatibility Matrix

Protocol compatibility should be a first-class test layer. Model type decides the default protocol set, but the CLI should allow explicit protocol selection because many channels support multiple client-facing APIs for the same upstream model.

### Protocol CLI Behavior

Default command:

```bash
fy-benchmark --channel-id 42 --model claude-sonnet-4-5 --type text --mode standard
```

Explicit protocol command:

```bash
fy-benchmark \
  --channel-id 42 \
  --model claude-sonnet-4-5 \
  --type text \
  --protocol chat \
  --protocol claude_messages \
  --mode strict
```

Rules:

- `--protocol` may be repeated or comma-separated.
- If omitted, use model type defaults.
- `--protocol-mode default` tests the canonical protocol plus low-cost compatibility smoke where safe.
- `--protocol-mode required` fails the run if any explicitly requested protocol is unsupported or broken.
- `--protocol-mode all` tests every protocol known for that type and reports unsupported paths separately from broken paths.
- Unsupported-but-not-required protocols should be marked `unsupported`, not `failed`.
- Required protocols that return unexpected 5xx, leak internal errors, or silently route to the wrong format should be `failed`.

### Text / Chat Models

Canonical and optional protocols:

| Protocol alias | Route | Purpose | Default |
|---|---|---|---|
| `chat` | `POST /v1/chat/completions` | OpenAI-compatible chat completion | Yes |
| `completion` | `POST /v1/completions` | Legacy OpenAI text completion compatibility | Optional |
| `responses` | `POST /v1/responses` | OpenAI Responses API compatibility | Optional unless model/client requires it |
| `responses_compact` | `POST /v1/responses/compact` | Responses compaction compatibility | Strict/deep only |
| `claude_messages` | `POST /v1/messages` | Claude Messages / CC-compatible clients | Default for Claude/Anthropic/AWS Bedrock Claude-family models; optional for others |
| `gemini_native` | `POST /v1beta/models/{model}:generateContent` | Gemini native request format | Default for Gemini-family native tests |
| `gemini_stream` | `POST /v1beta/models/{model}:streamGenerateContent` | Gemini native streaming compatibility | Strict/deep for Gemini-family models |

Text protocol tests should cover:

- Non-streaming success response.
- Streaming success response when the protocol supports stream.
- Request validation and safe 4xx behavior.
- No Go/internal struct leakage.
- Usage field presence and basic token sanity where expected.
- Tool/function-call pass-through for protocols that support tools.
- Reasoning/thinking fields for models that expose them.
- Cross-protocol response normalization: content, finish reason, usage, and model fields should be coherent.

### Image Models

Canonical and optional protocols:

| Protocol alias | Route | Purpose | Default |
|---|---|---|---|
| `image_generation` | `POST /v1/images/generations` | OpenAI-compatible image generation | Yes |
| `image_edit` | `POST /v1/images/edits` | OpenAI-compatible image edit | Default only for edit-capable models |
| `image_edit_legacy` | `POST /v1/edits` | Legacy edit route still wired to image relay | Strict/deep only |
| `image_variation` | `POST /v1/images/variations` | Not implemented; should return stable not-implemented semantics | Optional negative test |

Image protocol tests should cover:

- Generation with default prompt.
- Size, quality, output format, response format, and `n` handling.
- URL output and `b64_json` output validation.
- Edit request with multipart/image input or URL input where the model supports edit.
- Edit timeout behavior and useful error logging.
- Unsupported parameters return stable 4xx, not 5xx.
- Not-implemented routes return stable not-implemented errors.

### Video Models

Canonical and optional protocols:

| Protocol alias | Route | Purpose | Default |
|---|---|---|---|
| `openai_videos` | `POST /v1/videos`, `GET /v1/videos/{task_id}` | OpenAI-compatible video lifecycle | Yes |
| `video_generations` | `POST /v1/video/generations`, `GET /v1/video/generations/{task_id}` | Existing Fy-api video generation lifecycle | Optional |
| `video_content` | `GET /v1/videos/{task_id}/content` | Proxy generated video content | Strict/deep |
| `video_remix` | `POST /v1/videos/{video_id}/remix` | Remix lifecycle | Optional when model supports remix |
| `kling_video` | `/kling/v1/videos/text2video`, `/kling/v1/videos/image2video` | Kling official-compatible routes | Only for Kling-compatible model/channel tests |
| `jimeng_official` | `POST /jimeng/` | Jimeng official mapping | Only for Jimeng-compatible model/channel tests |

Video protocol tests should cover:

- Submit returns task ID.
- Fetch reaches success/failure terminal state with expected schema.
- Content proxy returns accessible video bytes for success tasks.
- Duration/resolution/fps validation should be added with `ffprobe` in strict/deep.
- Submit latency and completion latency should be separated.
- Unsupported lifecycle actions should fail as 4xx or explicit not-supported, not 5xx.

### Embedding / Audio / Rerank / Moderation

These are separate model types or auxiliary capabilities and should not be silently tested for every text model.

| Protocol alias | Route | Purpose |
|---|---|---|
| `embedding` | `POST /v1/embeddings` | Embedding model compatibility |
| `audio_transcription` | `POST /v1/audio/transcriptions` | Speech-to-text compatibility |
| `audio_translation` | `POST /v1/audio/translations` | Speech translation compatibility |
| `audio_speech` | `POST /v1/audio/speech` | Text-to-speech compatibility |
| `rerank` | `POST /v1/rerank` | Rerank model compatibility |
| `moderation` | `POST /v1/moderations` | Moderation compatibility |

The first implementation can register these aliases but only run them when explicitly requested with `--type embedding|audio|rerank|moderation` or `--protocol`.

### Protocol Scoring

Protocol compatibility should feed the `compliance` dimension, and in strict mode should also create blocking findings.

Suggested protocol status model:

- `pass`: expected route works.
- `unsupported`: route is not expected for this model/channel and failed cleanly.
- `not_implemented`: route is wired to a stable not-implemented handler.
- `fail`: expected route failed, leaked internals, returned unstable 5xx, or returned invalid schema.
- `skipped`: config missing required sample asset, token, or baseline.

Protocol scoring should separate:

- Required protocol pass rate.
- Optional protocol pass rate.
- Negative route correctness.
- Internal-error leak rate.
- Schema validity rate.

Strict mode should mark a required protocol `fail` as `BLOCKING`.

### Protocol Weighting

Protocol scoring should distinguish between high-traffic common client protocols and lower-priority legacy/native compatibility routes.

Common client-facing protocols should carry the highest default weight:

- `chat`: `POST /v1/chat/completions`
- `responses`: `POST /v1/responses`
- `image_generation`: `POST /v1/images/generations`
- `image_edit`: `POST /v1/images/edits`
- `claude_messages`: `POST /v1/messages`

Weighting rules:

- If the model belongs to the protocol's native vendor family, that native/common protocol is required and high weight.
  - OpenAI/Azure OpenAI text models: `chat` and `responses` are required/high weight.
  - OpenAI/Azure OpenAI image models: `image_generation`; `image_edit` is required only for edit-capable image models.
  - Anthropic/AWS Bedrock Claude-family models: `claude_messages` is required/high weight; `chat` remains important as gateway compatibility.
  - Gemini/Vertex models: `gemini_native` is required for native Gemini tests, but common OpenAI-compatible gateway routes should still have meaningful weight because customers often call Gemini models through OpenAI-compatible clients.
- If the model belongs to another vendor family, common OpenAI-compatible routes still carry high compatibility weight, while vendor-native or old legacy routes carry lower weight unless explicitly requested.
- Old/legacy protocols should not dominate the grade unless required by the test command or model profile.
- `unsupported` on an optional old protocol should not hurt the score; `fail` on a required/common protocol should hurt strongly and become `BLOCKING` in strict mode.

Suggested default weights inside the protocol-compliance subscore:

| Protocol class | Examples | Weight |
|---|---|---:|
| Common OpenAI-compatible text | `chat`, `responses` | 35% each for text models when applicable |
| Common OpenAI-compatible image | `image_generation`, `image_edit` | 45% / 35% for image models when applicable |
| Claude common/native | `claude_messages` | 35% for Claude-family text models |
| Vendor native modern | `gemini_native`, `gemini_stream` | 20-35% depending on model family |
| Current video lifecycle | `openai_videos`, `video_content` | 50% / 25% for video models when applicable |
| Legacy compatibility | `completion`, `responses_compact`, `image_edit_legacy`, `video_generations` | 5-15% |
| Not-implemented negative routes | `image_variation`, unsupported lifecycle actions | advisory, not weighted unless broken semantics leak/5xx |

For cross-vendor compatibility, use a cap instead of full failure:

- Example: Gemini model tested through `chat`/`responses`.
  - `chat` and `responses` should be high weight because they are common customer entry points.
  - `gemini_native` should also be tested, but a native issue should not outweigh broken OpenAI-compatible routes unless the run explicitly uses `--protocol-mode required` or the model profile marks native Gemini as required.
- Example: Claude model tested through `/v1/messages`.
  - `/v1/messages` failure is blocking.
  - `/v1/chat/completions` failure is also important because CC/OpenAI-compatible client routing is customer-facing, but the report should distinguish "native protocol failure" from "gateway compatibility failure".

Reports should show protocol failures grouped by severity:

- `Required common protocol failure`
- `Required native protocol failure`
- `Common compatibility weakness`
- `Legacy compatibility weakness`
- `Clean unsupported optional route`

## Parameter Compatibility Matrix

Protocol coverage alone is not enough. Each protocol test should also include parameter compatibility because most real gateway regressions happen when clients send legal-but-provider-specific fields, unsupported fields, wrong types, multipart inputs, or edge values.

### Parameter Test Modes

- `param_smoke`: one happy-path request with common parameters.
- `param_common`: common client parameters across mainstream SDKs.
- `param_strict`: broader boundary/value/type compatibility and provider-specific fields.
- `param_negative`: malformed or unsupported values should return stable 4xx/not-supported, not 5xx.

### Text Parameters

OpenAI-compatible chat and responses:

- Generation controls: `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stop`, `n`, `seed`.
- Penalties: `presence_penalty`, `frequency_penalty`, `repetition_penalty` where accepted.
- Streaming: `stream`, `stream_options.include_usage`.
- Output shape: `response_format`, JSON object/schema requests, plain text.
- Tools: `tools`, `tool_choice`, `parallel_tool_calls`, function-call argument schema.
- Reasoning: `reasoning_effort`, `thinking`, `include_reasoning`, provider-specific reasoning flags.
- Metadata/client fields: `user`, `metadata`, `store`, `service_tier`, `modalities`.
- Message content shapes: plain string, text part array, image input part, empty/null/malformed content.

Claude `/v1/messages`:

- Required fields: `messages`, `max_tokens`, `model`.
- Anthropic-specific fields: `system`, `temperature`, `top_p`, `top_k`, `stop_sequences`.
- Tools: `tools`, `tool_choice`.
- Extended fields: `thinking`, `anthropic_beta`, `context_management`, `cache_control`.
- Bedrock-sensitive fields must be explicitly tested because Bedrock rejects some Anthropic-native extensions.

Gemini native:

- `contents`, `parts`, `generationConfig`, `safetySettings`, `tools`, `systemInstruction`.
- Streaming and non-streaming actions.
- Image/multimodal parts when the model supports them.

### Image Parameters

Image generation:

- `prompt`, `model`, `n`.
- `size`, `quality`, `style`, `background`, `moderation`.
- `response_format`, `output_format`, `output_compression`.
- `user` and harmless metadata-like fields when accepted.
- Unsupported values should fail as stable 4xx.

Image edit:

- `image` as multipart file, base64, and URL where supported.
- `mask` optional/required behavior.
- `prompt`, `size`, `quality`, `n`, `response_format`.
- Large image timeout behavior and error observability.

### Video Parameters

OpenAI-compatible video:

- `prompt`, `model`.
- `seconds`/`duration`, `size`/`resolution`, `fps`, `aspect_ratio`.
- Image-to-video/reference inputs: image URL, base64, metadata/content arrays.
- Provider-specific fields: `watermark`, `camera_fixed`, `seed`, `guidance_scale`, `negative_prompt` when supported by channel.
- Fetch parameters and content download route.

Video parameter tests should also validate normalized task response fields:

- task ID
- status
- model
- created/completed timestamp when present
- progress
- output/content URL
- proxied content URL

### Parameter Scoring

Parameter compatibility should feed compliance and, in strict mode, produce weakness findings:

- Required happy-path parameter fails: `BLOCKING`.
- Common SDK parameter causes 5xx/internal leak: `BLOCKING`.
- Provider-specific unsupported parameter returns clean 4xx: `pass`.
- Optional parameter ignored with stable 2xx/4xx: `advisory`, unless required by model profile.
- Wrong type, malformed JSON, or bad multipart should never expose internal Go structs or panic traces.

The final report should show a parameter matrix per protocol:

| Protocol | Parameter Suite | Pass | Fail | Unsupported | Internal Leak | Blocking |
|---|---:|---:|---:|---:|---:|---:|

## Region And Environment Routing

Benchmark runs should test channels from the environment closest to their provider/customer traffic path.

### Defaults

- Domestic China models/channels should run against `cn-test`.
- Overseas/HK/international models/channels should run against `hk-test`.
- `--region cn|hk` explicitly selects the environment.
- `--region auto` infers from model/provider metadata and falls back to a clear error if ambiguous.

### Region Inference

Initial `auto` inference can use model keywords plus optional YAML overrides:

- CN examples: Qwen, Doubao, DeepSeek, Kimi, Zhipu, Baidu, Hunyuan, MiniMax, Step, Yi, Wan, Seedance, Kling, Jimeng.
- HK/overseas examples: GPT/OpenAI/Azure OpenAI, Claude/Anthropic/AWS Bedrock Claude, Gemini/Vertex/Google, Sora, Veo, Imagen, Mistral, Grok/xAI.

If a channel ID has explicit region metadata in the admin API, that should override keyword inference.

### Region Safety Rules

- Refuse `auto` when the model/provider cannot be classified.
- Include region and base URL in every report header.
- Include a warning if a domestic model is tested against HK or an overseas model against CN unless `--allow-cross-region` is provided.
- Keep output directories region-scoped, e.g. `benchmark-results/cn/ch42/...` and `benchmark-results/hk/ch42/...`.

## Deterministic Quality Behavior

By default, quality should not call an LLM judge. That means `fy-quality` should run only graders that can be checked locally or with non-judge infrastructure:

- `exact`
- `regex`
- `contains`
- `json_schema`

If `--with-embedding` is enabled, it can also run:

- `similarity`

If `--with-judge` is enabled, it can additionally run:

- `rubric`
- `pairwise`

Expected differences when judge is disabled:

- Lower cost and much faster runs.
- Better reproducibility because deterministic graders do not vary between judge models.
- Better for basic regression, protocol sanity, structured output, math, factual one-liners, and instruction-following checks with clear expected outputs.
- Weaker coverage for open-ended quality such as summarization, nuanced helpfulness, safety style, reasoning quality, writing quality, translation naturalness, and image prompt adherence.
- Final quality score should be labeled `deterministic_only` so reports do not overstate semantic quality.

Recommended default: deterministic-only in `standard`; full judge-based grading only in `deep` or when `--with-judge` is explicitly set.

In `strict`, deterministic quality should remain judge-free by default, but the dataset should prefer harder deterministic cases:

- More structured-output checks.
- More instruction-following checks.
- More exact/regex cases with narrow acceptable formats.
- More edge cases around multilingual prompts, JSON-only responses, and short-answer discipline.

This keeps strict mode fast and reproducible while making it better at exposing practical shortcomings.

## Canary And Baseline Behavior

Canary should not invent a baseline. If no baseline is configured, authenticity should be marked as unavailable or low-confidence rather than treated as proof of authenticity.

Default behavior without baseline:

- Text:
  - Run stateless probes only when useful: metadata and tokenizer fingerprint.
  - Skip alignment, drift, and MMD because they require trusted baseline samples.
  - Scorecard should show `authenticity: unavailable` or `low_confidence_stateless_only`.
- Image:
  - Run fingerprint and capability probes if configured.
  - Run cross-channel comparison only if extra channels are configured.
  - Skip vendor comparison if no vendor or baseline channel exists.
- Video:
  - No authenticity score initially unless a future video canary implementation exists.

Baseline options:

1. `--baseline-channel-id`: use another Fy-api channel as trusted baseline.
   - The runner should first create a baseline config pinned to this channel.
   - Then run audit pinned to the target channel.
   - Baseline source name must be stable per `(model, baseline_channel, target_channel)` to avoid mixing historical data.
2. Vendor direct config in YAML:
   - Use official vendor API where available.
   - Best for high-confidence authenticity.
3. Existing baseline file:
   - Reuse if fresh enough.
   - Respect existing `baseline_max_age_days`.

Important scoring rule: if canary is unavailable, do not set authenticity to zero by default. Treat it as unavailable and re-normalize active score dimensions. If canary fails with a real mismatch, authenticity should be zero and should add a suspected model-swap flag.

## Unified Scoring

Extend scoring to all model types.

### Text Weights

- Availability: 15%
- Performance: 25%
- Quality: 25%
- Authenticity: 20%
- Compliance: 15%

Availability gate: below 80% means final grade is F.

### Image Weights

- Availability: 20%
- Performance: 30%
- Quality: 20%
- Authenticity: 15%
- Compliance: 15%

Availability gate: below 90% means final grade is F.

### Video Weights

Initial video scoring should be conservative because current video tests are shallow.

- Availability: 35%
- Performance: 35%
- Quality: 10%
- Authenticity: 5%
- Compliance: 15%

Initial available inputs:

- Availability: submit success rate, fetch completion success rate.
- Performance: submit latency, completion time if smoke/fetch is enabled.
- Quality: unavailable until video output validation exists.
- Authenticity: unavailable until video canary exists.
- Compliance: basic API lifecycle correctness and error semantics.

Until quality/authenticity are implemented for video, reports must clearly label video grades as `limited_surface`.

### Plus/Minus Grade Bands

Replace coarse A/B/C/D/F bands with:

- `A+`: 97-100
- `A`: 93-96.99
- `A-`: 90-92.99
- `B+`: 87-89.99
- `B`: 83-86.99
- `B-`: 80-82.99
- `C+`: 77-79.99
- `C`: 73-76.99
- `C-`: 70-72.99
- `D+`: 67-69.99
- `D`: 63-66.99
- `D-`: 60-62.99
- `F`: below 60

If an availability gate fails, final grade remains `F` regardless of numeric weighted score.

### Strict Mode Scoring Adjustments

Strict mode should not invent a separate scoring system. It should use the same dimensions and grade bands, but feed stricter module thresholds into the same scorecard.

Suggested strict adjustments:

- Text availability gate: raise from 80% to 95%.
- Image availability gate: raise from 90% to 95%.
- Video availability gate: require both submit success and fetch completion success where fetch is enabled.
- Text performance anchors:
  - TTFT p95 best/worst: 500ms / 2000ms instead of 500ms / 3000ms.
  - E2E p95 best/worst: 5s / 20s instead of 5s / 30s.
  - Throughput worst: 20 tok/s instead of 10 tok/s.
- Image performance anchors:
  - P95 worst: 45s instead of 60s.
  - P50 worst: 15s instead of 20s.
- Compliance:
  - Any protocol leak, 5xx on client-error cases, or safety boundary failure should be flagged as blocking.
- Integrity:
  - Token inflation, stream repackaging, cache integrity, and deterministic instability should be elevated from advisory to blocking when severe.

Strict report labels:

- `BLOCKING`: must fix before promotion/customer use.
- `MAJOR`: materially weak but may be acceptable with traffic limits.
- `MINOR`: non-blocking observation.

Strict mode should include a final "Recommendation" field:

- `promote`
- `promote_with_limits`
- `do_not_promote`
- `needs_retest`

## Timing And Observability

Write a JSONL timeline:

```json
{"event":"run_start","ts":"2026-07-05T15:30:00+08:00","channel_id":42}
{"event":"model_start","model":"gpt-image-2","type":"image","ts":"..."}
{"event":"module_start","model":"gpt-image-2","module":"image_load","ts":"..."}
{"event":"module_end","model":"gpt-image-2","module":"image_load","duration_sec":612.4,"status":"pass","outputs":["...json","...md"]}
{"event":"model_end","model":"gpt-image-2","duration_sec":1380.2,"grade":"B+"}
{"event":"run_end","duration_sec":2760.9}
```

Each module wrapper should also capture:

- command executed
- return code
- stdout/stderr path
- start/end timestamp
- duration
- result files discovered
- failure reason if any

This makes it possible to tell whether time was spent in orchestration, gateway requests, image generation, judge calls, embedding calls, or report generation.

## Reproducibility And Run Control

Borrow the strongest operational patterns from established evaluation and benchmark tools: declarative configs, immutable run manifests, resumable execution, result comparison, and explicit budgets.

### Preflight Checks

Before running expensive tests, the orchestrator should run a preflight phase:

- Resolve final region and base URL.
- Verify token can call the selected environment.
- Verify channel pinning works for `channel_id`.
- Verify target model is visible through the selected channel/region.
- Verify optional judge and embedding models only when their modules are enabled.
- Verify required local fixtures exist for image edit, video image-to-video, audio, and multipart tests.
- Verify optional dependencies such as `ffprobe`, image-canary extras, or canary MMD extras only when needed.

Preflight should fail early with actionable errors. It should not start paid load or quality tests if the basic target model/channel is not reachable.

### Run Manifest

Every run should emit a redacted immutable manifest:

```json
{
  "run_id": "cn-ch42-20260705-153000",
  "git_commit": "...",
  "tool_version": "fy-channel-qa 0.5.0",
  "region": "cn",
  "base_url": "https://api-test.tracenex.cn",
  "channel_id": 42,
  "models": ["gpt-image-2"],
  "mode": "strict",
  "protocols": ["image_generation", "image_edit"],
  "config_hash": "...",
  "datasets": [{"name": "quality_deterministic", "version": "..."}],
  "secrets": "redacted"
}
```

This keeps reports comparable over time and makes it clear exactly what was tested.

### Resume And Rerun

Long benchmark runs should be resumable:

- `--resume RUN_DIR`: continue incomplete modules.
- `--rerun-failed`: rerun only failed modules.
- `--only-module load,conformance`: run a focused subset.
- `--only-model MODEL`: rerun one model inside a multi-model plan.
- `--force`: ignore existing module outputs and rerun.

Module state should be written after each module completes, not only at the end of the whole run.

### Budget Controls

Add explicit safeguards:

- `--max-duration-minutes`
- `--max-requests`
- `--max-cost-quota`
- `--fail-fast-on-blocking`
- `--cooldown-on-429`
- `--request-timeout-sec`
- `--module-timeout-sec`

Dry-run should estimate the planned request count by module, model, protocol, and parameter suite before any paid traffic is sent.

### Result Comparison

The runner should support comparing a new run to a prior run:

```bash
fy-benchmark compare \
  --current benchmark-results/cn/ch42/20260705-153000 \
  --baseline benchmark-results/cn/ch42/20260628-120000
```

Comparison should highlight:

- Grade movement.
- Availability and success-rate changes.
- P50/P95/P99 latency deltas.
- 429/5xx/timeout changes.
- Protocol/parameter regressions.
- New blocking findings.
- Cost/runtime changes.

This is useful for supplier retesting, post-fix verification, and weekly channel health tracking.

### Dataset And Fixture Versioning

Datasets and fixtures should be versioned and named in reports:

- Deterministic quality dataset version.
- Parameter compatibility matrix version.
- Protocol compatibility matrix version.
- Image edit fixture version: `scripts/channel-benchmark/fixtures/images/edit-source-256.png` and `edit-mask-256.png`.
- Image/video reference fixture version: `scripts/channel-benchmark/fixtures/images/reference-square-128.jpg`.
- Video fixture version: `scripts/channel-benchmark/fixtures/videos/reference-160x90-1s.mp4`.
- Audio fixture version: `scripts/channel-benchmark/fixtures/audio/tone-440hz-400ms.wav`.
- Private dataset indicator without leaking private prompt text.

Reports should distinguish public starter suites from private/customer-approved suites so public prompt memorization does not get mistaken for real quality.

## Open-Source Benchmark Patterns To Borrow

Useful patterns observed from open-source evaluation/benchmark ecosystems:

- GuideLLM / LLMPerf style: focus on real inference latency, throughput, concurrency, and percentile reporting.
- lm-evaluation-harness / OpenAI Evals style: declarative tasks, versioned datasets, repeatable scoring, and clear task metadata.
- HELM style: holistic reporting with scenarios, metrics, metadata, and transparent limitations.
- promptfoo style: CLI-first workflows, YAML-driven test cases, assertions, comparisons, and CI-friendly output.
- DeepEval / Ragas style: optional judge-based quality metrics, but with clear separation from deterministic checks.
- PromptBench style: robustness and prompt-variation tests that can expose brittle model/channel behavior.

The TraceNex runner should remain gateway/channel-oriented rather than becoming a generic model leaderboard. The primary goal is operational channel quality: protocol compatibility, parameter compatibility, latency, reliability, billing/usage sanity, authenticity, and safe failure behavior.

## Sprint 1: Orchestrator Skeleton

**Goal**: Add a runnable CLI that resolves config, expands models, creates run directories, and records timing without changing existing tool internals.

**Demo/Validation**:

- `fy-benchmark --config benchmark.example.yaml --channel-id 1 --model gpt-4o-mini --type text --mode quick --dry-run`
- Verify resolved execution plan and output directory structure.

### Task 1.1: Add Package And CLI Entry

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/`, `scripts/channel-benchmark/py/pyproject.toml`
- **Description**: Add `fy_benchmark` package and register `fy-benchmark`.
- **Dependencies**: None
- **Acceptance Criteria**:
  - CLI parses flags listed above.
  - `--dry-run` prints resolved models/modules.
  - Package is included in hatch build target.
- **Validation**:
  - `fy-benchmark --help`
  - Existing pytest suite still imports.

### Task 1.2: Config Loader

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/config.py`
- **Description**: Load YAML with env interpolation and merge CLI overrides.
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - Secrets can be read from env vars.
  - CLI channel/model values override YAML defaults.
  - Missing required values fail with actionable errors.
- **Validation**:
  - Unit tests for YAML-only, CLI override, missing token, missing model.

### Task 1.3: Timeline Writer

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/timeline.py`
- **Description**: Append structured timing events to `run.timeline.jsonl`.
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - Events are valid JSON lines.
  - Module start/end captures duration and status.
- **Validation**:
  - Unit test writes and parses timeline file.

### Task 1.4: Preflight And Manifest

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/preflight.py`, `scripts/channel-benchmark/py/fy_benchmark/manifest.py`
- **Description**: Add preflight checks and redacted run manifest generation.
- **Dependencies**: Tasks 1.1 and 1.2
- **Acceptance Criteria**:
  - Fails before paid tests when token/channel/model is invalid.
  - Writes `run.manifest.json` with secrets redacted.
  - Captures region, base URL, channel ID, models, mode, protocols, git commit, config hash, and dataset versions.
- **Validation**:
  - Unit tests for successful preflight, missing token, missing model, and redaction.

### Task 1.5: Resume State

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/state.py`
- **Description**: Track module state for resume/rerun behavior.
- **Dependencies**: Task 1.3
- **Acceptance Criteria**:
  - Supports `--resume`, `--rerun-failed`, `--only-module`, `--only-model`, and `--force`.
  - Does not rerun completed modules unless forced.
- **Validation**:
  - Unit tests with fake module states.

## Sprint 2: Module Wrappers

**Goal**: Generate temporary tool configs and run existing benchmark tools as subprocess modules.

**Demo/Validation**:

- Run one text model through quick mode and produce raw module outputs plus timeline.

### Task 2.1: Text Module Wrappers

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/modules/text.py`
- **Description**: Wrap smoke/load/quality/conformance/integrity/canary config generation and execution.
- **Dependencies**: Sprint 1
- **Acceptance Criteria**:
  - Quick mode runs without judge or embedding.
  - Standard mode runs deterministic quality only by default.
  - Deep mode requires explicit judge/canary dependencies or skips with clear reason.
  - Protocol and parameter compatibility suites can be generated per selected protocol.
- **Validation**:
  - Mock subprocess tests verify commands/configs.
  - One local dry-run fixture.

### Task 2.2: Image Module Wrappers

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/modules/image.py`
- **Description**: Wrap image load, image conformance, and optional image canary.
- **Dependencies**: Sprint 1
- **Acceptance Criteria**:
  - Image reports are emitted per model.
  - Expensive image canary is opt-in unless deep mode and required config are present.
  - Generation/edit parameter suites are separated so edit-capable models are not confused with generation-only models.
- **Validation**:
  - Mock subprocess tests.
  - Verify output file discovery for image result JSON.

### Task 2.3: Video Module Wrappers

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/modules/video.py`
- **Description**: Wrap current video smoke/load runners via `fy-eval` or direct internal runner calls.
- **Dependencies**: Sprint 1
- **Acceptance Criteria**:
  - Video models produce smoke/load report.
  - Final report marks video score as `limited_surface`.
  - OpenAI-compatible `/v1/videos` and legacy `/v1/video/generations` can be tested separately.
- **Validation**:
  - Mock tests for submit/load config.

## Sprint 3: Scoring Extensions

**Goal**: Generate unified scorecards for text, image, and video with plus/minus grades.

**Demo/Validation**:

- Run `fy-score` or orchestrator score aggregation over fixture outputs and verify `B+`, `B`, `B-` style grades.

### Task 3.1: Plus/Minus Grade Bands

- **Location**: `scripts/channel-benchmark/py/fy_score/scorer.py`
- **Description**: Replace or extend `GRADE_BANDS` with plus/minus bands.
- **Dependencies**: None
- **Acceptance Criteria**:
  - Existing scorecards use expanded grade labels.
  - Gate failures still force `F`.
- **Validation**:
  - Unit tests for boundary scores.

### Task 3.2: Video Scorecard

- **Location**: `scripts/channel-benchmark/py/fy_score/scorer.py`, `scripts/channel-benchmark/py/fy_score/loader.py`
- **Description**: Add video metric loading and `build_video_scorecard`.
- **Dependencies**: Task 2.3
- **Acceptance Criteria**:
  - Video availability/performance/compliance can be scored.
  - Missing quality/authenticity are unavailable, not zero.
  - Report shows `limited_surface`.
- **Validation**:
  - Fixture-based score tests.

### Task 3.3: Orchestrator Score Aggregation

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/scoring.py`
- **Description**: Collect module outputs and invoke score aggregation per model and channel.
- **Dependencies**: Tasks 3.1 and 3.2
- **Acceptance Criteria**:
  - Per-model scorecard JSON/Markdown.
  - Channel summary Markdown sorted by model and grade.
  - Protocol and parameter compatibility findings feed compliance and strict-mode weakness sections.
- **Validation**:
  - Integration test with fixture result files.

### Task 3.4: Run Comparison

- **Location**: `scripts/channel-benchmark/py/fy_benchmark/compare.py`
- **Description**: Compare current run against a previous run and report regressions/improvements.
- **Dependencies**: Task 3.3
- **Acceptance Criteria**:
  - Shows grade, latency, success-rate, protocol, parameter, and blocking-finding deltas.
  - Emits JSON and Markdown comparison reports.
- **Validation**:
  - Fixture-based comparison tests.

## Sprint 4: Documentation And Example Configs

**Goal**: Make the runner usable without reading code.

**Demo/Validation**:

- A user can copy the example YAML, set env vars, pass channel/model flags, and run quick mode.

### Task 4.1: Example YAML

- **Location**: `scripts/channel-benchmark/py/benchmark.example.yaml`
- **Description**: Add fully commented example config explaining every token and when it is used.
- **Dependencies**: Sprint 1
- **Acceptance Criteria**:
  - Explains admin token requirement for channel pinning.
  - Explains judge and embedding tokens are optional.
  - Explains baseline channel vs vendor direct.
  - Explains CN/HK region routing and how `--region auto` classifies models.
  - Explains budget controls and dry-run request estimation.
  - References committed fixture paths for image edit, image-to-video, audio, and video content tests.
- **Validation**:
  - Config loads with defaults and placeholder env vars.

### Task 4.1b: Fixture Assets

- **Location**: `scripts/channel-benchmark/fixtures/`
- **Description**: Commit tiny deterministic image, audio, and video fixtures plus a regeneration script.
- **Dependencies**: None
- **Acceptance Criteria**:
  - Includes source image, mask image, reference image, short WAV, and short MP4.
  - Includes `fixtures/README.md` explaining intended use and regeneration.
  - Fixtures contain no customer data, real people, secrets, or provider-generated outputs.
- **Validation**:
  - Regeneration script runs locally.
  - File sizes remain small enough for Git.

### Task 4.1c: Environment Installer

- **Location**: `scripts/channel-benchmark/install-env.sh`, `scripts/channel-benchmark/py/pyproject.toml`
- **Description**: Add a single setup script for the benchmark Python
  environment and fixture regeneration.
- **Dependencies**: Task 4.1b
- **Acceptance Criteria**:
  - Creates `py/.venv` with `uv`.
  - Installs the base editable package plus a lightweight `fixtures` extra by
    default.
  - Keeps heavy extras opt-in via `--with-canary`, `--with-image-canary`,
    `--with-tiktoken`, and `--with-dev`.
  - Checks Go availability without making Go mandatory for Python-only runs.
  - Regenerates fixtures unless `--skip-fixtures` is passed.
- **Validation**:
  - `bash -n scripts/channel-benchmark/install-env.sh`
  - `scripts/channel-benchmark/install-env.sh --help`
  - Fixture generator runs inside the created venv.

### Task 4.2: Runbook

- **Location**: `scripts/channel-benchmark/RUNBOOK-unified-benchmark.md`
- **Description**: Add operator workflow for quick/standard/deep runs.
- **Dependencies**: Sprints 1-3
- **Acceptance Criteria**:
  - Includes text/image/video examples.
  - Includes how to interpret deterministic-only quality and missing canary.
  - Includes timing analysis examples.
- **Validation**:
  - Manual review.

### Task 4.3: Overlay Entry

- **Location**: `OVERLAY.md`
- **Description**: Update B-7 benchmark entry with the new orchestrator and scoring changes.
- **Dependencies**: Implementation tasks
- **Acceptance Criteria**:
  - Notes new CLI, plus/minus score bands, video limited scoring, and timing logs.
- **Validation**:
  - Review for merge-friendly overlay wording.

## Testing Strategy

- Unit tests for config merging, mode expansion, timeline writing, grade bands, and video scorecard.
- Unit tests for preflight, manifest redaction, resume state, budget enforcement, and run comparison.
- Mock subprocess tests for text/image/video module wrappers.
- Fixture-based aggregation tests using small fake module outputs.
- Fixture-based regression comparison tests.
- Existing benchmark test suite: `cd scripts/channel-benchmark/py && pytest`.
- Focused CLI smoke:

```bash
fy-benchmark --config benchmark.example.yaml --channel-id 1 --model test-model --type text --mode quick --dry-run
fy-score --dry-run --loadtest-dir fixtures/loadtest --image-loadtest-dir fixtures/image
```

## Potential Risks And Gotchas

- **Token ambiguity**: Channel pinning requires admin user API key. The YAML and error messages must make this explicit.
- **Baseline trust**: A baseline channel is only as trustworthy as its upstream provider. Reports should say "baseline channel comparison" rather than "vendor verified" unless vendor direct config is used.
- **Judge disabled by default**: Deterministic quality is fast but narrower. Reports must not imply it measured open-ended reasoning quality.
- **Video scoring is immature**: Initial video grades have limited coverage. The report must show which dimensions are unavailable.
- **Strict mode may look harsher than official benchmark claims**: This is intended. Strict mode evaluates gateway/channel operational quality, not just model leaderboard ability.
- **Result discovery**: Existing tools write timestamped files. The orchestrator should capture files created during each module window, not glob the entire output directory blindly.
- **Parallel mode quota contention**: Keep model concurrency at 1 by default. Warn when `--parallel-models > 1`.
- **Image tests are expensive**: Deep image canary should stay opt-in or require deep mode plus explicit config.
- **Parameter matrix can explode**: Keep `quick` and `standard` bounded. Strict/deep can run larger suites, but every parameter case should be tagged by protocol and requirement level.
- **Region inference can be wrong**: Let YAML and CLI override auto routing. Reports must show final region decision and why it was chosen.
- **Different deployments may not have the same channel IDs**: Region-specific base URL plus channel ID must be treated as a pair.
- **Resuming stale runs can hide config drift**: Resume should compare the current config hash with the manifest hash and require `--force` on mismatch.
- **Benchmark comparison can be noisy**: Use thresholds for material regressions, not every tiny percentile movement.
- **Public datasets are weak evidence**: Public starter suites should be treated as wiring/regression checks, not definitive model-quality proof.

## Open Confirmations

1. Should `standard` mode include `fy-integrity` by default for all text models, or only for candidate channels before promotion?
2. For baseline via `--baseline-channel-id`, should the runner always rebuild baseline per run, or reuse baseline until `baseline_max_age_days` expires?
3. For video, should the first implementation only score submit/fetch behavior, or should it immediately download output and validate duration/resolution with `ffprobe`?

## Rollback Plan

- The new orchestrator is additive. If it causes issues, remove the `fy-benchmark` entry point and `fy_benchmark/` package without changing existing individual tools.
- Plus/minus grade bands can be reverted to coarse bands by restoring `GRADE_BANDS`.
- Video scorecard can be disabled by excluding video result files from aggregation.
