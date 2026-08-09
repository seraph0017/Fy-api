# Plan: Media Billing for Images and Videos

**Generated**: 2026-06-15  
**Estimated Complexity**: High

## Overview
Goal: unify billing for image generation and async video tasks without adding a separate payment system.

Recommended shape:
- keep the existing quota pipeline
- add one shared media pricing resolver
- write structured billing metadata into logs
- keep video task settlement as pre-consume + completion delta
- make image billing consistent across `/v1/images/*` and Responses `image_generation`

## Upstream Findings
- Upstream has already moved in the same direction for media billing:
  - PR #5035 adds per-resolution image model billing.
  - Issue #3426 asks for Gemini image billing to match official pricing.
  - PR #5300 adds Seedance 2.0 billing by output resolution, and notes that the hard-coded price map can be replaced once video supports expression billing.
  - PR #5387 extends Seedance billing to a two-dimensional model: output resolution × whether the request contains video input, then settles again using the response's actual resolution.
- The useful upstream pattern is not "everything must be expression billing." It is:
  - use normalized request/provider fields to produce billing dimensions
  - multiply against a base model price or rate
  - settle async video again from authoritative response fields
  - expose structured details in refund/usage logs

## Recommendation
1. Do not split media billing into a new subsystem.
2. Add one reusable media pricing layer that returns:
   - unit price
   - normalized size / quality / duration
   - fallback marker
   - source model / provider
   - explicit unknown-mode error or warning when the model is not priced
3. For images:
   - support model-specific billing modes instead of one global rule
   - fixed-per-image mode: quality + size select unit price, `n` is the multiplier
   - per-resolution mode: normalized resolution selects a multiplier or fixed unit price
   - token/rate mode: use upstream usage when it exposes image input/output tokens or provider-native usage
   - treat `auto` / missing params as explicit fallback metadata, not as silent defaults
   - never let an unpriced model silently settle at zero
4. For Responses image tool calls:
   - use the same image billing resolver as direct image endpoints
   - add the image-generation surcharge on top of normal model usage when the upstream response includes it
5. For async video:
   - continue using `OtherRatios` and `BillingContext`
   - pre-consume conservatively from requested duration / resolution
   - settle on completion using actual duration, output resolution, video-input flag, or provider-native usage
   - refund or supplement by delta
6. Keep all media billing evidence in `other` JSON so operators can audit it later.

## Sprint 1: Contract and pricing design
**Goal**: define one media billing contract that covers image and video paths.

### Task 1.1: Inventory billing entry points
- **Location**: `service/text_quota.go`, `service/tool_billing.go`, `relay/channel/openai/relay_responses.go`, `relay/image_handler.go`, `relay/relay_task.go`, `service/task_billing.go`, `service/task_polling.go`
- **Description**: map every place media cost can enter the system and mark which path owns final settlement.
- **Dependencies**: none
- **Acceptance Criteria**:
  - direct image, Responses image tool, and video task paths are all identified
  - each path has a single owner for final billing
- **Validation**:
  - short design note in the plan review

### Task 1.2: Define shared media billing result shape
- **Location**: new file, likely `service/media_billing.go` or `setting/operation_setting/media_pricing.go`
- **Description**: introduce a struct for normalized media billing output.
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - one struct can represent image and video billing
  - it includes fallback and source metadata
- **Validation**:
  - unit tests for the new struct helpers

### Task 1.3: Confirm upstream/vendor pricing rules
- **Location**: docs only
- **Description**: verify current public docs and upstream issues before finalizing exact defaults.
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - the plan cites official docs and upstream issue context
- **Validation**:
  - source list attached to the implementation PR

## Sprint 2: Shared pricing layer
**Goal**: centralize image and video price resolution.

### Task 2.1: Add image billing resolver
- **Location**: `setting/operation_setting/media_pricing.go` or equivalent
- **Description**: resolve the image billing mode and metadata for a model. Fixed-price image models return a unit price from `quality + size`; per-resolution models return a resolution multiplier or unit price; token/rate image models return the rate family and wait for upstream usage.
- **Dependencies**: Sprint 1
- **Acceptance Criteria**:
  - fixed-price models map `low/medium/high` and known sizes cleanly
  - per-resolution models normalize provider size/resolution names before pricing
  - token/rate models are not accidentally charged as fixed-price images
  - `auto` or unsupported values fall back predictably and are logged
  - `n` remains outside the resolver and acts as a multiplier only in fixed-price paths
  - unpriced models produce an explicit error, warning, or config gate instead of silent zero billing
- **Validation**:
  - table-driven unit tests for fixed-price, token/rate, known, and fallback cases

### Task 2.2: Add video ratio resolver
- **Location**: `setting/ratio_setting/model_ratio.go` or a new media pricing helper
- **Description**: resolve per-second, output-resolution, and input-media-based ratios for video models.
- **Dependencies**: Sprint 1
- **Acceptance Criteria**:
  - requested duration and resolution are normalized
  - video-input presence can participate in billing when the provider prices it differently
  - provider-specific rules remain isolated
- **Validation**:
  - unit tests for 720p / 1080p / fallback behavior

### Task 2.3: Add structured billing metadata helper
- **Location**: `service/task_billing.go`, `service/text_quota.go`
- **Description**: make sure media-specific pricing metadata lands in `other`.
- **Dependencies**: Task 2.1, 2.2
- **Acceptance Criteria**:
  - logs include normalized quality/size/duration
  - logs include fallback flags
- **Validation**:
  - log-shape unit tests

## Sprint 3: Image billing integration
**Goal**: make image billing consistent across direct image APIs and Responses tool calls.

### Task 3.1: Wire direct image endpoints to shared resolver
- **Location**: `relay/image_handler.go`, `service/text_quota.go`
- **Description**: use the shared image billing result for `/v1/images/generations` and `/v1/images/edits`. Preserve current model-price behavior where the model is configured as fixed price, and avoid double-counting token/rate-priced models.
- **Dependencies**: Sprint 2
- **Acceptance Criteria**:
  - `quality`, `size`, `n` are billed once for fixed-price models
  - token/rate models use provider usage rather than synthetic fixed-price constants
  - fallback metadata is logged
- **Validation**:
  - `service/text_quota_test.go`
  - image request unit tests

### Task 3.2: Wire Responses `image_generation` tool billing
- **Location**: `relay/channel/openai/relay_responses.go`, `service/text_quota.go`, `service/tool_billing.go`
- **Description**: reuse the same image billing resolver when the model emits an image-generation tool call. Charge a fixed image-generation surcharge only when the emitted tool/model matches a fixed-price image model; otherwise store provider usage and rate metadata for token/rate settlement.
- **Dependencies**: Sprint 2
- **Acceptance Criteria**:
  - Responses image generation has the same bill shape as direct image generation
  - mainline token usage and image cost remain separate and auditable
  - partial streamed images are not counted as extra final images
- **Validation**:
  - response handler tests
  - regression for upstream Responses image-generation behavior

### Task 3.3: Preserve provider quirks
- **Location**: `relay/channel/openai/adaptor.go`, `relay/channel/openai/adaptor_image_test.go`
- **Description**: keep current provider compatibility rules like Azure `response_format` handling.
- **Dependencies**: Task 3.1, 3.2
- **Acceptance Criteria**:
  - no regression in Azure GPT image behavior
  - no duplicate image charge on repeated render or partial image flows
- **Validation**:
  - provider-specific tests

## Sprint 4: Video billing integration
**Goal**: keep video billing on the existing async task path and make settlement exact.

### Task 4.1: Keep conservative pre-consume rules
- **Location**: `relay/relay_task.go`, `relay/channel/task/*/adaptor.go`
- **Description**: compute pre-consume from requested duration and resolution before task submission.
- **Dependencies**: Sprint 2
- **Acceptance Criteria**:
  - requested seconds and resolution are captured in `OtherRatios`
  - pre-charge is high enough to cover most completions
- **Validation**:
  - task submit unit tests

### Task 4.2: Settle on completion using actual usage
- **Location**: `service/task_polling.go`, `service/task_billing.go`
- **Description**: when the provider returns actual duration, output resolution, video-input metadata, token total, or usage, use it for final settlement and apply a refund or supplement.
- **Dependencies**: Task 4.1
- **Acceptance Criteria**:
  - exact duration-based settlement works
  - response-resolution settlement overrides request-side guesses when available
  - per-call skip does not block adaptor settlement
  - negative-balance regressions are prevented
- **Validation**:
  - regression for async video delta settlement
  - ali / seedance task tests

### Task 4.3: Preserve async billing audit data
- **Location**: `service/task_billing.go`, `model/task.go`
- **Description**: persist task-level billing context, request IDs, upstream request IDs, and final media settlement shape.
- **Dependencies**: Task 4.2
- **Acceptance Criteria**:
  - refund logs can be reconciled to the original task
  - task logs carry structured seconds/resolution fields
- **Validation**:
  - refund-log unit tests
  - task log reconciliation regression check

## Sprint 5: Docs, e2e, and rollout
**Goal**: make the new billing behavior testable and operable.

### Task 5.1: Update overlay and operator docs
- **Location**: `OVERLAY.md`, `docs/reports/*`, any operator runbook used by ops
- **Description**: document the billing contract, fallback behavior, and model-specific caveats.
- **Dependencies**: Sprint 3, Sprint 4
- **Acceptance Criteria**:
  - new media billing behavior is documented in one place
  - operators know where to inspect logs and settle deltas
- **Validation**:
  - doc review only

### Task 5.2: Extend e2e coverage
- **Location**: `scripts/ops/media_billing_e2e.py`
- **Description**: cover direct image, Responses image tool, and video task billing.
- **Dependencies**: Sprint 3, Sprint 4
- **Acceptance Criteria**:
  - script verifies logs, quota, and task terminal states
  - script can run on cn-test and staging
- **Validation**:
  - dry run + live run against test env

### Task 5.3: Rollout and observe
- **Location**: deployment workflow only
- **Description**: deploy to test, check logs, then promote after operator sign-off.
- **Dependencies**: Task 5.1, 5.2
- **Acceptance Criteria**:
  - no zero-charge regression
  - no unexpected media billing drift
- **Validation**:
  - production smoke check

## Testing Strategy
- Unit tests for image price resolution, fallback behavior, and `n` multiplication.
- Unit tests for video duration/resolution settlement and refund/supplement paths.
- Regression tests for:
  - Responses image generation billing
  - direct image billing
  - async video negative-balance settlement
  - refund log reconciliation
- E2E on cn-test / staging using public APIs and log queries.

## Potential Risks & Gotchas
- `gpt-image-2` pricing is token/rate based, while older OpenAI image-generation behavior and some downstream vendors use fixed per-image prices. The resolver must choose mode by model/provider.
- `auto` quality and missing `size` need explicit fallback rules or billing will be inconsistent.
- Silent zero pricing is dangerous; recent upstream issues already show models like `gpt-image-2` landing at `quota=0` when model pricing is missing.
- Some providers return output tokens that do not match the true image cost; do not charge image generation from output tokens alone.
- Async video tasks can finish above the pre-consume estimate; completion settlement must stay atomic.
- Request-side video resolution is not authoritative. Upstream PR #5387 explicitly fixes bypass cases by settling from response resolution.
- Base-price coupling is risky: if code computes multipliers as `actual/base`, operator docs or validation must make sure the configured base `ModelRatio` matches the code's expected base.
- Refund logs are created later, after the HTTP request context is gone, so request IDs must be preserved in task metadata.
- Provider quirks like `response_format`, `tool_choice`, and `input_reference` vs `input.media` should stay isolated to adapter code.

## Rollback Plan
- Remove the shared media pricing helper.
- Restore the prior image and video billing call sites.
- Keep existing task log tables and settlement tables unchanged.
- Preserve the old behavior until the next approved iteration.

## Sources
- [OpenAI image generation guide](https://developers.openai.com/api/docs/guides/image-generation)
- [OpenAI GPT Image 2 model](https://developers.openai.com/api/docs/models/gpt-image-2)
- [OpenAI image generation tool](https://developers.openai.com/api/docs/guides/tools-image-generation)
- [OpenAI video generation guide](https://developers.openai.com/api/docs/guides/video-generation)
- [OpenAI pricing page](https://openai.com/api/pricing/)
- [QuantumNous/new-api issue #3426](https://github.com/QuantumNous/new-api/issues/3426)
- [QuantumNous/new-api PR #5035](https://github.com/QuantumNous/new-api/pull/5035)
- [QuantumNous/new-api PR #5300](https://github.com/QuantumNous/new-api/pull/5300)
- [QuantumNous/new-api PR #5387](https://github.com/QuantumNous/new-api/pull/5387)
