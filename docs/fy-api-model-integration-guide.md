# TraceNex / Fy-api 主流模型接入指南

> 版本：v1.0  
> 更新日期：2026-06-15  
> 面向对象：客户开发者、售前/交付、运营、管理员  
> 本文说明如何通过 TraceNex / Fy-api 统一网关接入主流模型。页面中的模型名是代表性示例，最终可用模型以平台模型广场和管理员分配为准。

## 1. 文档定位

这不是厂商原生 API 文档，也不是简单的接口清单。它的目标只有一个：

1. 让客户知道 Fy-api 统一怎么接。
2. 让运营知道新模型怎么挂到平台里。
3. 让客户在最短路径内选对协议、填对参数、拿到结果。

## 2. 快速结论

调用方式：

- 文本、图片、Embedding、Rerank、语音转文字：通常是同步接口。
- Chat / Responses / Claude / Gemini：可按模型能力选择同步或流式。
- 视频、Midjourney、Suno：异步任务，先提交，再按 task id 查询。

鉴权方式：

```http
Authorization: Bearer {FY_API_KEY}
Content-Type: application/json
```

核心接口：

```text
文本对话：POST /v1/chat/completions
Responses：POST /v1/responses
Claude：POST /v1/messages
Gemini：POST /v1beta/models/{model}:generateContent
图片生成：POST /v1/images/generations
视频提交：POST /v1/videos
视频查询：GET  /v1/videos/{task_id}
视频下载：GET  /v1/videos/{task_id}/content
Embedding：POST /v1/embeddings
Rerank：POST /v1/rerank
语音：POST /v1/audio/transcriptions、/v1/audio/translations、/v1/audio/speech
模型列表：GET /v1/models、GET /v1beta/models
```

推荐客户优先使用：

- OpenAI SDK：`base_url = {BASE_URL}/v1`，优先走 `/v1/chat/completions`。
- Anthropic SDK：走 `/v1/messages`。
- Gemini SDK / 原生 Gemini 调用：走 `/v1beta/models/{model}:generateContent`。
- 视频生成：统一走 `/v1/videos`，不要把视频提交接口当同步接口使用。

## 3. 平台统一接入规则

### 3.1 Base URL

客户调用时使用部署地址作为 Base URL：

```text
https://你的网关域名
```

OpenAI 兼容 SDK 一般填：

```text
https://你的网关域名/v1
```

### 3.2 鉴权

所有对外模型调用使用 Bearer Token：

```http
Authorization: Bearer sk-你的令牌
Content-Type: application/json
```

令牌由用户在“令牌管理”创建。令牌可以绑定分组、额度、过期时间、模型权限和 IP 白名单。

### 3.3 模型名

`model` 必须填写平台模型广场或管理员提供的精确模型名。模型名错误、令牌无权限、分组无可用渠道，都会导致请求失败。

### 3.4 request_id

每次请求会生成请求 ID。客户报错时应提供：

- 请求时间
- 模型名
- 端点
- HTTP 状态码
- 响应里的错误内容
- `request_id` 或响应头里的 `X-Oneapi-Request-Id`

管理员可在用量日志中按 `request_id` 检索。

## 4. 协议与端点总览

| 能力 | 推荐端点 | 协议 | 典型模型/厂商 | 返回方式 |
|---|---|---|---|---|
| 模型列表 | `GET /v1/models`、`GET /v1beta/models` | OpenAI / Gemini | 平台当前可见模型 | 同步 |
| 文本对话 | `POST /v1/chat/completions` | OpenAI Chat Completions | GPT、DeepSeek、Qwen、Doubao、Moonshot、智谱、OpenRouter、xAI、Ollama 等 | 同步或 SSE 流式 |
| 旧补全文本 | `POST /v1/completions` | OpenAI Completions | 旧模型、FIM 类模型 | 同步 |
| 新版多模态/工具 | `POST /v1/responses` | OpenAI Responses | OpenAI/Codex/部分兼容渠道 | 同步或流式 |
| Responses 压缩 | `POST /v1/responses/compact` | OpenAI Responses Compaction | Codex/Responses 相关模型 | 同步 |
| Claude 原生 | `POST /v1/messages` | Anthropic Messages | Claude、Bedrock Claude、Vertex Claude、部分 Moonshot/通义 Claude 兼容 | 同步或流式 |
| Gemini 原生 | `POST /v1beta/models/{model}:generateContent` | Gemini Native | Gemini、Vertex Gemini | 同步 |
| Gemini 流式 | `POST /v1beta/models/{model}:streamGenerateContent?alt=sse` | Gemini Native SSE | Gemini、Vertex Gemini | SSE |
| 图片生成 | `POST /v1/images/generations` | OpenAI Image | DALL-E、GPT Image、通义万相、火山、腾讯、Replicate 等 | 同步或任务式，由模型决定 |
| 图片编辑 | `POST /v1/images/edits` | OpenAI Image Edit | GPT Image、部分图片编辑模型 | 同步 |
| 视频生成 | `POST /v1/videos` 或 `POST /v1/video/generations` | OpenAI-like Video Task | Sora、Veo、Kling、Vidu、Seedance、Wanx、Hailuo 等 | 异步任务 |
| 视频查询 | `GET /v1/videos/{task_id}` | OpenAI-like Video Task | 视频任务 | 查询任务 |
| 视频内容代理 | `GET /v1/videos/{task_id}/content` | Video Proxy | 视频任务 | 二进制视频 |
| 语音转文字 | `POST /v1/audio/transcriptions` | OpenAI Audio | Whisper、兼容 ASR | 同步 |
| 翻译 | `POST /v1/audio/translations` | OpenAI Audio | Whisper、兼容翻译 | 同步 |
| 语音合成 | `POST /v1/audio/speech` | OpenAI Audio | TTS、火山语音、MiniMax 等 | 音频 |
| Embedding | `POST /v1/embeddings` | OpenAI Embeddings | OpenAI、Jina、Cohere、通义、Ollama 等 | 同步 |
| Rerank | `POST /v1/rerank` | Rerank | Jina、Cohere、通义、硅基流动等 | 同步 |
| Realtime | `GET /v1/realtime` | OpenAI Realtime WebSocket | Realtime 兼容模型 | WebSocket |
| Midjourney | `/mj/*` | Midjourney Proxy | MJ 绘图 | 异步任务 |
| Suno | `/suno/*` | Suno Task | Suno 音乐 | 异步任务 |

## 5. 协议参数字典

### 5.1 OpenAI Chat Completions

端点：

```text
POST /v1/chat/completions
```

适用：大多数文本对话、多模态图片理解、工具调用、JSON 输出、流式输出。

必填参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | 模型名 |
| `messages` | array | 对话消息数组 |

常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `stream` | boolean | 是否开启 SSE 流式输出 |
| `stream_options.include_usage` | boolean | 流式末尾是否包含 usage，取决于渠道支持 |
| `temperature` | number | 随机性 |
| `top_p` | number | nucleus sampling |
| `max_tokens` | integer | 最大输出 token |
| `max_completion_tokens` | integer | 新模型推荐的最大输出字段 |
| `response_format` | object | JSON object / JSON schema 输出 |
| `tools` | array | 函数/工具调用 |
| `tool_choice` | string/object | 工具选择策略 |
| `reasoning_effort` | string | 推理强度，常见值 `low` / `medium` / `high`，是否生效取决于模型 |
| `seed` | number | 可复现随机种子，取决于上游支持 |
| `web_search_options` | object | 搜索上下文，`search_context_size` 支持 `low` / `medium` / `high` |

多模态图片输入示例：

```json
{
  "model": "gpt-4o-mini",
  "messages": [
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "请描述这张图片" },
        {
          "type": "image_url",
          "image_url": {
            "url": "https://example.com/image.png"
          }
        }
      ]
    }
  ]
}
```

### 5.2 OpenAI Responses

端点：

```text
POST /v1/responses
```

适用：需要使用 Responses 协议的模型、Codex 类模型、新版工具/多模态工作流。

必填参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | 模型名 |
| `input` | string/array/object | 输入内容 |

常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `stream` | boolean | 是否流式 |
| `instructions` | string | 系统指令 |
| `tools` | array | 工具 |
| `reasoning` | object | 推理配置 |
| `text` | object | 文本输出配置 |
| `metadata` | object | 元数据 |

### 5.3 Claude Messages

端点：

```text
POST /v1/messages
```

适用：Claude 原生协议、Anthropic SDK、Bedrock/Vertex Claude 兼容场景。

必填参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | Claude 模型名 |
| `messages` | array | 消息数组 |

常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `max_tokens` | integer | 最大输出 token |
| `system` | string/array | 系统提示 |
| `stream` | boolean | 是否流式 |
| `temperature` | number | 随机性 |
| `top_p` | number | nucleus sampling |
| `top_k` | integer | top-k |
| `stop_sequences` | array | 停止序列 |
| `tools` | array | Claude 工具 |
| `tool_choice` | object | 工具选择 |
| `thinking` | object | Claude thinking 配置，取决于模型支持 |
| `metadata.user_id` | string | 用户标识 |

注意：

- `context_management`、`speed`、`service_tier`、`inference_geo` 等高级字段默认可能会被网关过滤，只有管理员在渠道设置中允许后才会透传。
- Claude 图片输入使用 content block：`type=image` + `source`。

### 5.4 Gemini Native

端点：

```text
POST /v1beta/models/{model}:generateContent
POST /v1beta/models/{model}:streamGenerateContent?alt=sse
POST /v1beta/models/{model}:embedContent
POST /v1beta/models/{model}:batchEmbedContents
```

适用：Google Gemini 原生 SDK、需要 Gemini `contents` / `parts` 结构的客户。

必填参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `contents` | array | Gemini 内容数组 |

常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `systemInstruction` / `system_instruction` | object | 系统指令 |
| `generationConfig` | object | 生成配置 |
| `safetySettings` | array | 安全配置 |
| `tools` | array/object | 工具 |
| `toolConfig` | object | 工具配置 |
| `cachedContent` | string | Gemini 缓存内容 |

`generationConfig` 常用字段：

| 参数 | 说明 |
|---|---|
| `temperature` | 随机性 |
| `topP` / `topK` | 采样控制 |
| `maxOutputTokens` | 最大输出 |
| `responseMimeType` | 输出 MIME，如 `application/json` |
| `responseSchema` | JSON schema |
| `thinkingConfig` | Gemini thinking 配置，支持 `includeThoughts` / `thinkingBudget` |

### 5.5 图片生成与编辑

端点：

```text
POST /v1/images/generations
POST /v1/images/edits
```

必填参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | 图片模型名 |
| `prompt` | string | 提示词 |

常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `n` | integer | 生成数量，默认 1 |
| `size` | string | 尺寸，如 `1024x1024` |
| `quality` | string | 质量，如 `standard` / `hd` / `low` / `medium` / `high`，实际取决于模型 |
| `response_format` | string | `url` 或 `b64_json`，部分 Azure GPT Image 渠道不支持该字段，会被网关处理 |
| `style` | string | DALL-E 风格，取决于模型 |
| `background` | string | 背景，取决于模型 |
| `output_format` | string | 输出格式，取决于模型 |
| `watermark` | boolean | 是否加水印，取决于渠道 |
| `image` / `images` | string/array | 图片编辑或参考图 |
| `mask` | string | 编辑遮罩 |

尺寸校验：

- `dall-e-2` / `dall-e`：`256x256`、`512x512`、`1024x1024`
- `dall-e-3`：`1024x1024`、`1024x1792`、`1792x1024`
- 其他模型以对应上游支持为准。

### 5.6 视频任务

推荐端点：

```text
POST /v1/videos
GET /v1/videos/{task_id}
GET /v1/videos/{task_id}/content
```

兼容端点：

```text
POST /v1/video/generations
GET /v1/video/generations/{task_id}
POST /v1/videos/{video_id}/remix
```

适用：Sora、Veo、Kling、Vidu、豆包 Seedance、阿里 Wanx、MiniMax Hailuo 等。

常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | 视频模型名 |
| `prompt` | string | 提示词 |
| `image` | string | 图生视频首图 URL/Base64 |
| `image_tail` | string | 尾帧图，部分模型支持 |
| `images` | array | 多参考图，部分模型支持 |
| `reference_urls` | array | 阿里 `wan2.6-r2v*` 推荐参考素材字段 |
| `media` | array | 新版参考素材数组，元素可含 `type` / `url` / `reference_voice` |
| `duration` / `seconds` | number/string | 时长，单位秒 |
| `size` | string | 尺寸，如 `1280*720`、`1920*1080` |
| `resolution` | string | 分辨率，如 `720P`、`1080P` |
| `width` / `height` | integer | 宽高，部分模型支持 |
| `fps` | integer | 帧率，部分模型支持 |
| `seed` | integer | 随机种子 |
| `n` | integer | 生成数量，通常为 1 |
| `watermark` | boolean | 水印 |
| `audio` | boolean | 是否生成音频，取决于模型 |
| `metadata` | object | 厂商特有参数透传 |

任务状态：

| 状态 | 含义 |
|---|---|
| `queued` | 已排队 |
| `in_progress` | 生成中 |
| `completed` | 完成 |
| `failed` | 失败 |

轮询建议：5 秒后开始查询，随后 10 秒、20 秒、30 秒退避。不要每秒轮询。

### 5.7 Audio

端点：

```text
POST /v1/audio/transcriptions
POST /v1/audio/translations
POST /v1/audio/speech
```

语音转文字/翻译常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | ASR 模型 |
| `file` | multipart file | 音频文件，取决于客户端 |
| `response_format` | string | 默认 `json` |
| `language` | string | 语言，部分模型支持 |

语音合成常用参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | TTS 模型 |
| `input` | string | 待合成文本 |
| `voice` | string | 音色 |
| `instructions` | string | 风格指令，部分模型支持 |
| `response_format` | string | 输出格式 |
| `speed` | number | 语速 |
| `stream_format` | string | `sse` 时表示流式音频 |

### 5.8 Embedding

端点：

```text
POST /v1/embeddings
```

必填参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | Embedding 模型 |
| `input` | string/array | 输入文本 |

可选参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `encoding_format` | string | `float` / `base64`，取决于模型 |
| `dimensions` | integer | 输出维度，取决于模型 |
| `user` | string | 用户标识 |

### 5.9 Rerank

端点：

```text
POST /v1/rerank
```

必填参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | Rerank 模型 |
| `query` | string | 查询 |
| `documents` | array | 待重排文档 |

可选参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `top_n` | integer | 返回前 N 条 |
| `return_documents` | boolean | 是否返回原文档 |
| `max_chunk_per_doc` | integer | 每文档最大分块 |
| `overlap_tokens` | integer | 分块重叠 token |

## 6. 主流模型族怎么接

### 6.1 协议选择建议

| 客户场景 | 优先协议 | 说明 |
|---|---|---|
| OpenAI SDK / 大多数通用接入 | OpenAI Chat / Responses / Image / Audio / Embedding | 最统一，迁移成本最低 |
| Anthropic SDK / Claude 原生功能 | Claude Messages | 保留 Claude 原生消息和工具语义 |
| Gemini 原生 SDK / Veo | Gemini Native | 适合 `contents` / `parts` 结构 |
| 视频生成 | Video Task | 统一提交任务，再轮询查询 |
| 检索重排 | Rerank | 面向检索排序场景 |
| 本地/私有化模型 | Chat / Embedding | 只要兼容协议就能接入 |

| 模型/厂商 | 推荐协议 | 管理后台渠道类型 | 典型用途 | 备注 |
|---|---|---|---|---|
| OpenAI GPT / GPT Image / Realtime | Chat、Responses、Image、Audio、Realtime、Video(Sora) | OpenAI / Sora | 通用文本、多模态、图片、语音、视频 | OpenAI SDK 兼容度最高 |
| Azure OpenAI | Chat、Responses、Image、Audio | Azure | 企业 Azure 部署 | 需要配置 Azure endpoint 与 api-version |
| Claude | Claude Messages 或 Chat 兼容 | Anthropic / AWS / VertexAI | 长文本、代码、工具调用 | Anthropic SDK 建议走 `/v1/messages` |
| Gemini | Gemini Native 或 Chat 兼容 | Gemini / VertexAI | 多模态、长上下文、Veo | 原生 SDK 走 `/v1beta/models/*` |
| DeepSeek | Chat | DeepSeek / OpenAI 兼容渠道 | 推理、代码、中文 | 常用 `reasoning_effort` 或模型自带推理 |
| Qwen / 通义千问 / Wanx | Chat、Image、Video、Rerank、Embedding | Ali | 中文、多模态、图片视频 | DashScope 原生复杂参数建议放 `metadata` |
| Doubao / 火山 / Seedance | Chat、Image、Audio、Video | VolcEngine / DoubaoVideo | 中文、语音、视频 | 视频是异步任务 |
| Moonshot / Kimi | Chat、Claude 兼容、Embedding、Rerank | Moonshot | 长文本、中文 | 取决于渠道能力 |
| 智谱 GLM | Chat、Image | Zhipu / ZhipuV4 | 中文、多模态 | 部分图片字段不同 |
| MiniMax / Hailuo | Chat、Audio、Video | MiniMax | 语音、视频 | Hailuo 走视频任务 |
| Cohere / Jina | Rerank、Embedding | Cohere / Jina | 检索增强 | Rerank 走 `/v1/rerank` |
| OpenRouter | Chat、Responses | OpenRouter | 聚合模型 | 模型名通常带厂商前缀 |
| Ollama / Xinference | Chat、Embedding | Ollama / Xinference | 私有化模型 | Base URL 指向本地或内网服务 |
| Kling / Vidu / Sora / Veo | Video Task | Kling / Vidu / Sora / VertexAI/Gemini | 视频生成 | 统一用 `/v1/videos` |

## 7. 可复制调用示例

### 7.1 Chat Completions

```bash
curl -X POST https://api.example.com/v1/chat/completions \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      {"role": "system", "content": "你是一个简洁的助手。"},
      {"role": "user", "content": "用三句话介绍 TraceNex。"}
    ],
    "temperature": 0.7,
    "stream": false
  }'
```

### 7.2 流式 Chat

```bash
curl -N -X POST https://api.example.com/v1/chat/completions \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "写一个 Go HTTP server 示例"}],
    "stream": true,
    "stream_options": {"include_usage": true}
  }'
```

### 7.3 Claude Messages

```bash
curl -X POST https://api.example.com/v1/messages \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "system": "你是代码审查助手。",
    "messages": [
      {"role": "user", "content": "请审查这个函数是否有并发问题。"}
    ]
  }'
```

### 7.4 Gemini Native

```bash
curl -X POST "https://api.example.com/v1beta/models/gemini-2.5-pro:generateContent" \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "解释一下 RAG 的基本流程"}]
      }
    ],
    "generationConfig": {
      "temperature": 0.4,
      "maxOutputTokens": 1024
    }
  }'
```

### 7.5 图片生成

```bash
curl -X POST https://api.example.com/v1/images/generations \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-1",
    "prompt": "一张 TraceNex 风格的 API 网关架构图，干净、专业、深色背景",
    "size": "1024x1024",
    "quality": "high",
    "n": 1
  }'
```

### 7.6 视频生成

```bash
curl -X POST https://api.example.com/v1/videos \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wan2.6-r2v-flash",
    "prompt": "人物在咖啡馆窗边弹吉他，镜头缓慢推进，电影感",
    "reference_urls": [
      "https://example.com/character.png"
    ],
    "size": "1280*720",
    "duration": 5,
    "audio": true,
    "watermark": false
  }'
```

查询：

```bash
curl https://api.example.com/v1/videos/task_xxx \
  -H "Authorization: Bearer sk-你的令牌"
```

下载/代理播放：

```bash
curl -L https://api.example.com/v1/videos/task_xxx/content \
  -H "Authorization: Bearer sk-你的令牌" \
  --output result.mp4
```

### 7.7 Embedding

```bash
curl -X POST https://api.example.com/v1/embeddings \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-3-small",
    "input": ["第一段文本", "第二段文本"],
    "dimensions": 1024
  }'
```

### 7.8 Rerank

```bash
curl -X POST https://api.example.com/v1/rerank \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "jina-reranker-v2-base-multilingual",
    "query": "如何接入 Fy-api 视频模型？",
    "documents": [
      "视频模型使用 /v1/videos 异步提交。",
      "Embedding 使用 /v1/embeddings。",
      "用户需要先创建令牌。"
    ],
    "top_n": 2,
    "return_documents": true
  }'
```

### 7.9 Python SDK 示例

```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-你的令牌",
    base_url="https://api.example.com/v1",
)

resp = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello Fy-api"}],
)

print(resp.choices[0].message.content)
```

### 7.10 JavaScript SDK 示例

```javascript
import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk-你的令牌",
  baseURL: "https://api.example.com/v1",
});

const resp = await client.chat.completions.create({
  model: "gpt-4o-mini",
  messages: [{ role: "user", content: "Hello Fy-api" }],
});

console.log(resp.choices[0].message.content);
```

## 8. 管理员接入新模型流程

### 8.1 准备信息

接入前先准备：

- 厂商账号和 API Key
- Base URL 或区域信息
- 模型精确名称
- 模型能力：文本、图片、视频、音频、Embedding、Rerank
- 协议类型：OpenAI 兼容、Claude、Gemini、厂商任务协议
- 是否支持流式
- 是否支持工具调用
- 是否支持多模态输入
- 价格：token、图片张数、视频秒数、分辨率、任务次数

### 8.2 后台创建渠道

路径：管理员后台 -> 渠道管理 -> 添加渠道。

| 字段 | 填写建议 |
|---|---|
| 类型 | 选择对应厂商，如 OpenAI、Azure、Anthropic、Gemini、Ali、VolcEngine、Kling、Vidu |
| 名称 | 运营可读名称，如 `openai-prod-1` |
| 分组 | `default`、`vip`、`video-only` 等 |
| 密钥 | 厂商 key；多 key 可按系统支持方式配置 |
| Base URL | 留空走默认；代理或私有部署时填写 |
| 模型 | 精确模型名，多个模型按后台规则分隔 |
| 测试模型 | 选低成本模型，避免探活成本过高 |

特殊认证：

- Azure：配置 Azure endpoint、deployment、api-version。
- Vertex AI：通常需要 service account JSON 或 API key 模式，取决于渠道设置。
- AWS Bedrock：需要 AWS key、region、模型权限。
- Ali / VolcEngine / 腾讯等国内云：可能需要 AccessKey/SecretKey 或专用签名配置。

### 8.3 配置模型权限和价格

1. 在模型配置中确认模型可见。
2. 在模型倍率/模型价格中设置价格。
3. 文本模型按 token 计费。
4. 图片模型按张、尺寸、质量计费。
5. 视频模型按秒、分辨率、任务档位计费。
6. 给测试分组设置小额度，先跑通再放量。

### 8.4 验证

至少验证：

- `/v1/models` 能看到模型。
- `/v1/chat/completions` 或对应端点能成功。
- 流式输出正常。
- 用量日志有 usage 和费用。
- 错误时能在日志中查到 `request_id`。
- 视频任务能提交、查询、完成、下载。

## 9. 常见问题与排障

| 问题 | 可能原因 | 处理 |
|---|---|---|
| 401 Unauthorized | Token 错误或过期 | 重新创建令牌 |
| 403 Forbidden | 模型无权限、分组无权限、IP 白名单不匹配 | 检查令牌权限和用户分组 |
| 400 model is required | 请求体缺少 `model` | 补模型名 |
| 400 field messages is required | Chat 请求缺少 `messages` | 使用 Chat schema |
| 400 contents is required | Gemini 原生请求缺少 `contents` | 使用 Gemini schema |
| 429 Too Many Requests | 限流或余额不足触发保护 | 降低频率、检查额度 |
| 5xx | 上游异常、渠道不可用、代理异常 | 用 `request_id` 查日志，必要时切换渠道 |
| 视频一直 in_progress | 上游生成慢或轮询延迟 | 等待并按退避轮询 |
| 图片 size 报错 | 尺寸不在模型允许列表 | 改为模型支持尺寸 |
| 流式没有 usage | 渠道不支持 `stream_options.include_usage` | 以平台用量日志为准 |

## 10. 需要明确写在页面上的限制

- `/v1/files`、fine-tunes、图片 variations 等部分 OpenAI 端点当前返回 not implemented。
- 同名参数是否生效取决于模型和上游渠道，网关只保证接收和尽力转换。
- 高级字段可能被管理员配置过滤，防止不兼容或额外计费。
- 视频、Midjourney、Suno 等任务类接口不是同步生成，必须按任务查询。
- 价格展示和实际扣费以平台后台配置、模型倍率、分组倍率、任务实际用量为准。
