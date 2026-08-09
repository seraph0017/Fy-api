# DSPEC: Media Billing Dimensions for Images and Videos

**Status**: Implemented in `feature/media-billing`  
**Date**: 2026-06-15  
**Owner**: TraceNex / Fy-api  
**Scope**: image generation, Responses image-generation tool calls, async video generation tasks

## 1. Problem

Media models do not share one stable parameter schema.

Examples:
- one provider uses `quality=hd`, another uses `quality=high`
- one provider uses `size=1280x720`, another uses `resolution=720P`
- one provider prices by output tokens, another by image count, another by seconds and resolution
- async video responses can differ from requested params, so request-side billing is not authoritative

If billing reads raw provider fields directly, every model becomes a billing special case and reports become hard to audit.

## 2. Goals

- Keep one unified billing flow instead of a separate media payment system.
- Normalize provider/model-specific request fields into canonical billing dimensions.
- Keep provider-specific mapping inside adapters or model-family helpers.
- Support fixed image price, per-resolution image price, token/rate image price, and video duration/resolution pricing.
- Support async video completion settlement from authoritative response fields.
- Write structured billing evidence into `other` JSON for logs, refunds, and reports.
- Fail or warn explicitly when a priced media model is not configured; never silently settle at zero.

## 3. Non-Goals

- Do not make every provider use the same upstream request payload.
- Do not force all media pricing into `tiered_expr`.
- Do not expose every provider-native parameter as a first-class TraceNex field.
- Do not migrate existing text/audio billing logic unless needed for media integration.

## 4. Existing System

Relevant current paths:

- Direct image generation:
  - `relay/image_handler.go`
  - `service/text_quota.go`
  - `relay/helper/price.go`

- Responses API image tool calls:
  - `relay/channel/openai/relay_responses.go`
  - `service/text_quota.go`
  - `service/tool_billing.go`

- Async video tasks:
  - `relay/relay_task.go`
  - `relay/channel/task/*/adaptor.go`
  - `service/task_billing.go`
  - `service/task_polling.go`

- Dynamic expression billing:
  - `pkg/billingexpr/expr.md`
  - `relay/helper/price.go`
  - `service/tiered_settle.go`

Important existing behavior:

- Expression billing already supports `img` and `img_o` image token variables.
- Task/video billing already uses `OtherRatios` and `TaskBillingContext`.
- Task polling can call `AdjustBillingOnComplete` for final actual quota.

## 5. Upstream Direction

Upstream new-api is moving toward normalized media dimensions:

- `QuantumNous/new-api#3426`: Gemini image billing should match official pricing.
- `QuantumNous/new-api#5035`: adds per-resolution image model billing.
- `QuantumNous/new-api#5300`: Seedance 2.0 pricing by output resolution; notes future expression support could replace some hard-coded maps.
- `QuantumNous/new-api#5387`: Seedance billing by output resolution x video-input flag, then response-resolution settlement and refund reconciliation.

Design implication:

Use canonical billing dimensions and adapter mapping. Expression billing is useful, but not the only mechanism.

## 6. Design Principles

### 6.1 Raw Fields Are Not Billing Fields

Raw provider fields stay in adapters.

Billing should not read `parameters.size`, `imageConfig.aspectRatio`, `resolution`, `ratio`, `quality`, `output_format`, etc. directly from arbitrary raw JSON.

Adapters translate raw fields into canonical dimensions.

### 6.2 Map By Provider / Model Family, Not Every Model

Do not create a separate mapper for every model if a model family shares one protocol.

Preferred order:

1. Provider-family mapper, for example Ali video, Doubao/Seedance, OpenAI image, Gemini image.
2. Model-family override, for example `wan2.6-r2v*` vs `wan2.7-r2v*`.
3. Exact model override only when necessary.

### 6.3 Request Estimate Is Not Final Truth

For async tasks, request dimensions are used for pre-consume only.

Final settlement should prefer:

1. provider-native usage
2. response output resolution
3. response actual duration
4. response media metadata
5. request estimate as fallback

### 6.4 Expression Billing Is Optional

Use `tiered_expr` when the billing input is token-like or already normalized.

Do not make expression evaluate provider raw fields like `param("parameters.size") == "1280*720"` for every adapter. That recreates provider coupling in configuration.

Acceptable expression inputs:

- `img`, `img_o`
- normalized `media.duration_seconds`
- normalized `media.resolution_bucket`
- normalized `media.quality_bucket`
- normalized `media.has_video_input`

## 7. Canonical Billing Schema

Introduce a canonical media billing shape.

Recommended package:

- `service/media_billing.go` for service-level structs and helpers
- adapter-specific mapping remains in `relay/channel/...`

### 7.1 MediaBillingDimensions

```go
type MediaBillingDimensions struct {
    Modality string // "image" | "video"

    ModelName         string
    UpstreamModelName string
    Provider          string

    BillingMode string // "fixed_image" | "resolution_image" | "token_rate" | "video_duration" | "expression"

    ImageCount int
    QualityRaw string
    QualityBucket string // "low" | "medium" | "high" | "auto" | "unknown"

    SizeRaw string
    Width int
    Height int
    ResolutionBucket string // "480p" | "720p" | "1080p" | "2k" | "4k" | "square" | "custom" | "unknown"
    AspectRatio string // "1:1" | "16:9" | "9:16" | "4:3" | "3:4" | "custom" | "unknown"

    DurationSeconds float64

    HasImageInput bool
    HasVideoInput bool
    ReferenceImageCount int
    ReferenceVideoCount int

    ProviderUsage map[string]float64

    Fallbacks []string
    Warnings []string
}
```

### 7.2 MediaBillingEstimate

```go
type MediaBillingEstimate struct {
    Dimensions MediaBillingDimensions

    UnitPrice float64
    Unit string // "image" | "second" | "token_1m" | "request"

    Multiplier float64
    EstimatedQuota int

    OtherRatios map[string]float64
    Other map[string]any
}
```

### 7.3 Log `other` Shape

Every media log should carry:

```json
{
  "media_billing": true,
  "media_modality": "video",
  "media_billing_mode": "video_duration",
  "media_resolution_bucket": "720p",
  "media_quality_bucket": "high",
  "media_duration_seconds": 5,
  "media_image_count": 1,
  "media_has_video_input": false,
  "media_unit_price": 0.3,
  "media_unit": "second",
  "media_multiplier": 5,
  "media_fallbacks": ["quality:auto->high"],
  "media_provider_usage": {
    "duration": 5
  }
}
```

Keep existing fields like `seconds`, `resolution-1080P`, and `image_generation_call_price` during migration for compatibility. New fields should be additive first.

## 8. Mapping Rules

### 8.1 Quality

Canonical buckets:

- `low`
- `medium`
- `high`
- `auto`
- `unknown`

Examples:

| Raw | Canonical |
| --- | --- |
| `low` | `low` |
| `standard` | `medium` or provider-specific default |
| `hd` | `high` |
| `high` | `high` |
| empty | `auto` with fallback |

Provider-family mapper decides ambiguous values.

### 8.2 Resolution

Canonical buckets:

- `480p`
- `720p`
- `1080p`
- `2k`
- `4k`
- `square`
- `custom`
- `unknown`

Examples:

| Raw | Width | Height | Bucket |
| --- | ---: | ---: | --- |
| `720P` | 0 | 0 | `720p` |
| `1280*720` | 1280 | 720 | `720p` |
| `1280x720` | 1280 | 720 | `720p` |
| `1920x1080` | 1920 | 1080 | `1080p` |
| `1024x1024` | 1024 | 1024 | `square` |
| `1536x1024` | 1536 | 1024 | `custom` plus aspect `3:2` |

Rule:

- parse dimensions when possible
- otherwise map known aliases
- if neither works, bucket is `unknown` and billing must choose explicit fallback or error

### 8.3 Duration

Canonical field:

- `DurationSeconds float64`

Accepted raw fields:

- `duration`
- `seconds`
- provider-native usage duration

Rule:

- request duration is estimate
- response usage duration wins for final settlement
- zero or missing duration must not silently produce zero charge for video models

### 8.4 Media Input Flags

Canonical fields:

- `HasImageInput`
- `HasVideoInput`
- `ReferenceImageCount`
- `ReferenceVideoCount`

Purpose:

- some video providers price image-to-video and video-to-video differently
- upstream PR #5387 follows this pattern for Seedance

## 9. Billing Modes

### 9.1 `fixed_image`

Use for models priced by one generated image.

Formula:

```text
quota = unit_price * image_count * group_ratio * quota_per_unit
```

Inputs:

- image count
- quality bucket
- size or resolution bucket

### 9.2 `resolution_image`

Use for image models priced by generated resolution class.

Formula:

```text
quota = base_price * resolution_multiplier * image_count * group_ratio * quota_per_unit
```

or:

```text
quota = resolution_unit_price * image_count * group_ratio * quota_per_unit
```

Configuration must declare which formula the model uses.

### 9.3 `token_rate`

Use when upstream returns token-like usage for image input/output.

Formula:

```text
quota = expression_or_rate(img, img_o, p, c) * group_ratio
```

Preferred implementation:

- use existing `tiered_expr` for models that expose `img` / `img_o`
- keep direct image endpoint fixed-price logic out of token-rate models

### 9.4 `video_duration`

Use for async video priced by seconds and media dimensions.

Pre-consume:

```text
quota_estimate = base_second_price * requested_seconds * resolution_multiplier * input_media_multiplier * group_ratio * quota_per_unit
```

Completion:

```text
quota_actual = base_second_price * actual_seconds * actual_resolution_multiplier * actual_input_media_multiplier * group_ratio * quota_per_unit
delta = quota_actual - pre_consumed_quota
```

### 9.5 `expression`

Use when normalized media dimensions are enough for expression-based pricing.

Do not use raw provider fields directly.

Target future expression variables:

- `media.duration_seconds`
- `media.resolution_bucket`
- `media.quality_bucket`
- `media.image_count`
- `media.has_video_input`

This likely requires extending `billingexpr.RequestInput` or adding a typed media context.

## 10. Adapter Contract

Add an optional interface that adapters can implement.

```go
type MediaBillingMapper interface {
    BuildMediaBillingDimensions(c *gin.Context, info *relaycommon.RelayInfo) (service.MediaBillingDimensions, error)
}
```

For async task adapters, keep existing task billing hooks but make them return / persist canonical dimensions:

```go
type TaskMediaBillingMapper interface {
    EstimateMediaBilling(c *gin.Context, info *relaycommon.RelayInfo) (service.MediaBillingEstimate, error)
    ActualMediaBilling(task *model.Task, taskResult *relaycommon.TaskInfo) (service.MediaBillingEstimate, error)
}
```

Migration can be staged:

1. keep existing `EstimateBilling` / `AdjustBillingOnComplete`
2. internally build canonical dimensions
3. write canonical dimensions to `OtherRatios` and `other`
4. later replace raw `OtherRatios` construction with the new estimate object

## 11. Data Flow

### 11.1 Direct Image

```text
request
  -> image request validation
  -> adapter maps raw fields to MediaBillingDimensions
  -> price resolver selects image billing mode
  -> existing PreConsumeBilling / PostTextConsumeQuota
  -> log `other.media_*`
```

### 11.2 Responses Image Tool

```text
responses upstream response
  -> detect image_generation_call
  -> extract final image tool quality / size / count if available
  -> map to MediaBillingDimensions
  -> add image surcharge or token-rate settlement
  -> log separate media fields
```

Important:

- streamed partial images must not increase final image count
- final completed event or final response object is authoritative

### 11.3 Async Video

```text
request
  -> adapter maps request to estimated dimensions
  -> price resolver computes estimate
  -> PreConsumeBilling
  -> store TaskBillingContext + media dimensions snapshot
  -> submit upstream task
  -> polling fetches final response
  -> adapter maps response to actual dimensions
  -> RecalculateTaskQuota
  -> log delta with media fields
```

## 12. Configuration

Add a model media billing config. Exact storage can be DB option JSON initially.

Example:

```json
{
  "gpt-image-1": {
    "mode": "fixed_image",
    "default_quality": "high",
    "default_size": "1024x1024",
    "prices": {
      "low:1024x1024": 0.011,
      "medium:1024x1024": 0.042,
      "high:1024x1024": 0.167
    }
  },
  "gpt-image-2": {
    "mode": "token_rate",
    "expression_model": "gpt-image-2"
  },
  "wan2.6-i2v": {
    "mode": "video_duration",
    "base_resolution": "720p",
    "per_second_price": 0.3,
    "resolution_multipliers": {
      "720p": 1,
      "1080p": 1.666667
    }
  }
}
```

Open question:

- whether this belongs under `operation_setting`, `ratio_setting`, or a new `media_billing_setting`.

Recommendation:

- use a new `setting/media_billing_setting` package if the config grows beyond simple price maps
- keep existing `ratio_setting` reads for compatibility during migration

## 13. Migration Plan

### Phase 1: Add canonical structs and helpers

- Add `service/media_billing.go`.
- Add quality/resolution/duration normalization helpers.
- Add tests for common aliases.

No behavior change.

### Phase 2: Wire existing image billing metadata

- Keep current quota results.
- Add normalized fields to `other`.
- Make image generation fallback explicit.

Behavior change: log-only, except unpriced model warning if enabled.

### Phase 3: Wire video request dimensions

- Make Ali / Sora / Gemini / Vertex task adapters build canonical dimensions internally.
- Continue writing `OtherRatios`.
- Add `media_*` log fields.

Behavior change: mostly additive.

### Phase 4: Completion settlement from canonical actual dimensions

- Extend `AdjustBillingOnComplete` implementations to use actual dimensions.
- Prefer provider usage / response resolution.
- Keep old settlement as fallback.

Behavior change: actual quota can change when response differs from request.

### Phase 5: Optional expression integration

- Extend expression input with normalized media dimensions only if needed.
- Do not block image/video correctness on expression support.

## 14. Testing

### Unit Tests

- quality normalization:
  - `hd -> high`
  - empty -> `auto` fallback
  - invalid -> `unknown`

- resolution normalization:
  - `720P -> 720p`
  - `1280*720 -> 720p`
  - `1920x1080 -> 1080p`
  - `1024x1024 -> square`

- image billing:
  - fixed image price
  - per-resolution image price
  - token-rate model does not use fixed price
  - unpriced model errors or warns

- video billing:
  - request-side estimate
  - response-side actual duration override
  - response-side actual resolution override
  - video-input multiplier
  - refund delta
  - supplemental charge delta

### E2E

Extend `scripts/ops/media_billing_e2e.py`:

- `/v1/images/generations`
- Responses image-generation tool
- `/v1/video/generations`
- `/v1/videos`
- log query verifies `media_*` fields
- task completion verifies delta settlement

## 15. Rollout

1. Add canonical fields as log-only.
2. Run e2e on cn-test.
3. Compare old quota and new estimated quota in logs.
4. Enable actual canonical settlement for one low-risk model family.
5. Expand to Ali / Seedance / OpenAI image.
6. Update `OVERLAY.md` after behavior changes.

## 16. Risks

- **Wrong fallback price**: must log fallback and alert on unknown buckets.
- **Zero-charge regression**: unpriced media models must fail or warn.
- **Double billing**: token-rate image models must not also receive fixed-image surcharge.
- **Async mismatch**: request-side dimensions can differ from final output. Response settlement must win.
- **Config drift**: base price and multiplier must be validated together.
- **Report breakage**: keep legacy fields during migration.

## 17. Decision

Adopt canonical media billing dimensions.

Do not create per-model billing fields in service-level code.

Do create adapter/model-family mappers that translate raw provider fields into canonical dimensions.

Use expression billing only after normalization, not as a replacement for adapter mapping.
