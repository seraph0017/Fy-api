# fy-api 平台 Seedance 2.0 视频生成接入手册

面向通过 fy-api 平台调用 Seedance 2.0 的客户。fy-api 对外提供 OpenAI-like 视频任务接口，内部会转换为火山方舟 Seedance 2.0 官方异步任务协议。

## 快速结论

| 项目 | 说明 |
| --- | --- |
| 调用方式 | 异步任务 |
| 提交任务 | `POST /v1/videos` |
| 查询任务 | `GET /v1/videos/{task_id}` |
| 下载视频 | `GET /v1/videos/{task_id}/content` |
| 鉴权方式 | `Authorization: Bearer {FY_API_KEY}` |
| 推荐模型 | `doubao-seedance-2-0-260128`、`doubao-seedance-2-0-fast-260128` |

## 1. 接口地址

### 提交视频生成任务

```http
POST {BASE_URL}/v1/videos
Authorization: Bearer {FY_API_KEY}
Content-Type: application/json
```

### 查询任务状态

```http
GET {BASE_URL}/v1/videos/{task_id}
Authorization: Bearer {FY_API_KEY}
```

### 下载或代理访问视频内容

```http
GET {BASE_URL}/v1/videos/{task_id}/content
Authorization: Bearer {FY_API_KEY}
```

兼容旧路径：

```http
POST /v1/video/generations
GET  /v1/video/generations/{task_id}
```

建议新接入统一使用 `/v1/videos`。

## 2. 支持模型

| 模型 | 说明 |
| --- | --- |
| `doubao-seedance-2-0-260128` | Seedance 2.0 标准模型 |
| `doubao-seedance-2-0-fast-260128` | Seedance 2.0 Fast 模型 |

如果平台管理员配置了模型映射，客户仍按平台分配的模型名传参即可。

## 3. 文生视频示例

```bash
curl -X POST "{BASE_URL}/v1/videos" \
  -H "Authorization: Bearer {FY_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "prompt": "一只白色猫咪在阳光下的窗台上伸懒腰，镜头缓慢推进，电影感，真实摄影风格",
    "seconds": "5",
    "size": "1280x720"
  }'
```

提交成功后会立即返回一个平台任务 ID：

```json
{
  "id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "queued",
  "progress": 0,
  "created_at": 1760000000
}
```

## 4. 查询任务结果

```bash
curl "{BASE_URL}/v1/videos/task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "Authorization: Bearer {FY_API_KEY}"
```

处理中响应示例：

```json
{
  "id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "in_progress",
  "progress": 50,
  "created_at": 1760000000
}
```

成功响应示例：

```json
{
  "id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "completed",
  "progress": 100,
  "created_at": 1760000000,
  "completed_at": 1760000120,
  "metadata": {
    "url": "https://..."
  }
}
```

失败响应示例：

```json
{
  "id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "progress": 100,
  "error": {
    "code": "xxx",
    "message": "失败原因"
  }
}
```

## 5. 状态枚举

| 状态 | 含义 | 客户侧处理建议 |
| --- | --- | --- |
| `queued` | 排队中 | 继续轮询 |
| `in_progress` | 生成中 | 继续轮询 |
| `completed` | 已完成 | 读取 `metadata.url` 或调用 `/content` 下载 |
| `failed` | 失败 | 读取 `error.message` 排查 |
| `unknown` | 未知状态 | 稍后重试查询或联系平台 |

推荐轮询间隔：每 10-30 秒查询一次。

## 6. 请求字段说明

### 基础字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 模型名，例如 `doubao-seedance-2-0-260128` |
| `prompt` | string | 是 | 视频生成提示词，不能为空 |
| `seconds` | string | 建议 | 视频时长，例如 `"5"`、`"10"`；建议传字符串 |
| `size` | string | 否 | 输出尺寸，fy-api 会转换为 Seedance 的 `resolution` + `ratio` |
| `metadata` | object / string | 否 | 透传或扩展官方 Seedance 参数；可传 JSON 对象，也兼容 JSON 字符串 |

### size 映射规则

fy-api 对外使用 OpenAI-like 的 `size` 字段，内部转换为 Seedance 官方字段：

| `size` | 转换后 `resolution` | 转换后 `ratio` |
| --- | --- | --- |
| `1280x720` | `720p` | `16:9` |
| `720x1280` | `720p` | `9:16` |
| `1920x1080` | `1080p` | `16:9` |
| `1080x1920` | `1080p` | `9:16` |
| `720p` | `720p` | 默认 `16:9` |
| `1080p` | `1080p` | 默认 `16:9` |

如果不传 `size`，也不传 `metadata.resolution`，平台默认按 `720p`、`16:9` 处理。

注意：如果同时传 `size` 和 `metadata.resolution` / `metadata.ratio`，两者必须一致，否则会返回参数错误。

## 7. 图生视频和多参考图

fy-api 支持用顶层字段传参考图，平台会自动转换为官方 Seedance 的 `content` 数组，并设置：

```json
{
  "type": "image_url",
  "role": "reference_image"
}
```

### 单图示例

```bash
curl -X POST "{BASE_URL}/v1/videos" \
  -H "Authorization: Bearer {FY_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "prompt": "根据参考图生成一段人物在城市街头自然行走的视频，保持人物外观一致，真实摄影风格",
    "seconds": "5",
    "size": "1280x720",
    "image": "https://example.com/person.jpg"
  }'
```

### 多图示例

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "使用多张参考图生成产品展示视频，保持产品外观一致，镜头缓慢环绕",
  "seconds": "5",
  "size": "1280x720",
  "images": [
    "https://example.com/product-front.jpg",
    "https://example.com/product-side.jpg"
  ]
}
```

### 参考图字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `image` | string | 单张参考图 URL |
| `images` | string[] | 多张参考图 URL |
| `input_reference` | string | 兼容字段，会作为参考图处理 |

Seedance 2.0 官方支持多参考图，常见范围为 1-9 张，具体以模型和平台配置为准。

## 8. 参考视频和多模态素材

可以通过 `media` 传入参考图片或参考视频：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "参考视频的运动节奏和镜头语言，生成一段同风格产品广告",
  "seconds": "5",
  "size": "1280x720",
  "media": [
    {
      "type": "video",
      "url": "https://example.com/reference.mp4"
    }
  ]
}
```

`media` 字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `type` | string | `image` / `image_url` / `video` / `video_url` |
| `url` | string | 素材 URL |

fy-api 转换规则：

| `media.type` | 转换为官方 content 类型 | role |
| --- | --- | --- |
| `image` / `image_url` | `image_url` | `reference_image` |
| `video` / `video_url` | `video_url` | `reference_video` |

## 9. 透传 Seedance 官方字段

Seedance 官方参数可放在 `metadata` 中传入。fy-api 会把 `metadata` 反序列化后合并到官方请求体。

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "一段国风庭院中的人物走动视频，镜头固定，细节丰富",
  "seconds": "5",
  "size": "1280x720",
  "metadata": {
    "seed": 12345,
    "camera_fixed": true,
    "watermark": false,
    "generate_audio": true,
    "return_last_frame": true,
    "execution_expires_after": 172800
  }
}
```

常用 `metadata` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `resolution` | string | 官方分辨率，例如 `720p`、`1080p` |
| `ratio` | string | 官方画幅比例，例如 `16:9`、`9:16` |
| `duration` | integer | 官方时长字段；客户侧更推荐使用顶层 `seconds` |
| `frames` | integer | 官方帧数相关字段，按官方模型能力生效 |
| `seed` | integer | 随机种子 |
| `camera_fixed` | boolean | 是否固定镜头 |
| `watermark` | boolean | 是否添加水印 |
| `generate_audio` | boolean | 是否生成音频，官方默认通常为 `true` |
| `return_last_frame` | boolean | 是否返回末帧图，具体返回内容以官方能力为准 |
| `callback_url` | string | 官方回调地址；一般不建议客户依赖，推荐轮询 fy-api 查询接口 |
| `execution_expires_after` | integer | 任务过期时间，单位秒；官方默认 172800 秒 |
| `service_tier` | string | 官方服务档位；Seedance 2.0 官方说明仅支持在线推理，不建议修改 |
| `content` | array | 官方原生 content 数组，高级场景使用 |

注意：`prompt` 请始终放在顶层 `prompt` 字段。即使在 `metadata.content` 中传了 `text` 内容，fy-api 也会以顶层 `prompt` 作为最终文本提示词。

## 10. 官方 content 原生写法

高级客户可以通过 `metadata.content` 传官方 Seedance content 结构。

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "使用参考图生成一段自然走动视频，保持人物一致",
  "seconds": "5",
  "metadata": {
    "resolution": "720p",
    "ratio": "16:9",
    "content": [
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/person.jpg"
        },
        "role": "reference_image"
      }
    ]
  }
}
```

fy-api 会补齐未设置的参考素材 `role`：

| content 类型 | 自动补齐 role |
| --- | --- |
| `image_url` | `reference_image` |
| `video_url` | `reference_video` |

## 11. 真人参考图说明

Seedance 2.0 官方对包含真实人脸或真人的参考图有更严格要求。fy-api 已做平台侧适配：

当请求模型为 Seedance 2.0，并且参考图 URL 是 `http://` 或 `https://` 时，平台会自动进行可信素材准备，内部审核并转换为 `asset://...` 后再提交给 Seedance。

客户侧不需要直接传 `asset://`，仍然传普通图片 URL 即可：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "参考图中的人物在咖啡馆中自然微笑并转头看向窗外",
  "seconds": "5",
  "size": "1280x720",
  "image": "https://example.com/real-person.jpg"
}
```

这类任务可能先返回 `queued`，实际提交会在素材准备完成后继续进行。客户只需要按 `task_id` 轮询查询。

## 12. 1080p 说明

客户请求 `size = 1920x1080` 或 `1080x1920` 时，平台按 1080p 产品档处理。

在部分平台配置下，fy-api 可能会内部采用“Seedance 720p 生成 + 火山增强到 1080p”的 pipeline。该过程对客户透明：

- 客户仍按 1080p 请求和计费档处理；
- 查询接口只返回最终视频 URL；
- 内部 pipeline task id、增强状态等不会暴露给客户。

## 13. 完整示例

带音频、固定镜头、无水印、1080p 输出：

```bash
curl -X POST "{BASE_URL}/v1/videos" \
  -H "Authorization: Bearer {FY_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "prompt": "一段高端香水产品广告，产品放在黑色大理石台面上，水滴缓慢滑落，镜头固定，灯光高级，真实摄影风格",
    "seconds": "5",
    "size": "1920x1080",
    "image": "https://example.com/perfume.jpg",
    "metadata": {
      "camera_fixed": true,
      "watermark": false,
      "generate_audio": true,
      "seed": 123456
    }
  }'
```

## 14. 接入注意事项

- `prompt` 必填，不能为空。
- `seconds` 建议传字符串，例如 `"5"`，不要传空值。
- 推荐用 `size` 表达输出尺寸，除非确实需要官方原生参数。
- 不建议同时传 `size` 和冲突的 `metadata.resolution` / `metadata.ratio`。
- 顶层 `audio`、`watermark`、`reference_urls` 等字段不是 Seedance 当前推荐入口；Seedance 官方参数请放到 `metadata`。
- 参考图和参考视频必须是平台和上游可访问的 URL。
- 对真实人脸参考图，平台会自动走可信素材准备，耗时可能比普通任务更长。
- 视频生成是异步任务，请不要把提交接口当同步接口使用。

## 15. 官方参考

- 火山方舟 Seedance 2.0 API Reference：https://www.volcengine.com/docs/82379/1520757
- BytePlus ModelArk Seedance 2.0 API Reference：https://docs.byteplus.com/en/docs/ModelArk/1520757

