# 腾讯 VOD GPT Image 2 接入

腾讯 VOD 的 GPT Image 2 使用 VOD AIGC 异步任务协议，不是腾讯 AIArt，也不是 OpenAI 兼容上游。

## 渠道配置

| 字段 | 值 |
| --- | --- |
| 类型 | Tencent |
| Base URL | `https://gateway.vod-qcloud.com` |
| Key | `SubAppId|SecretId|SecretKey` |
| 模型 | `gpt-image-2` |
| 测试模型 | `gpt-image-2` |

不要把 Base URL 配成 `https://aiart.tencentcloudapi.com`。该地址使用另一套 `SubmitContentToImageGPTJob` 协议。

## 公共请求

```json
{
  "model": "gpt-image-2",
  "prompt": "一间位于山顶的玻璃茶室",
  "quality": "medium",
  "size": "1024x1024",
  "n": 1,
  "response_format": "url"
}
```

请求地址为 `POST /v1/images/generations`。网关内部映射为：

- `model` -> `ModelName=OG`
- `quality=low|medium|high` -> `ModelVersion=image2_low|image2_medium|image2_high`
- `quality=auto` -> `image2_medium`
- `n` -> `OutputConfig.OutputImageCount`，范围 1 到 8
- `size` -> `ExtInfo.AdditionalParameters.size`
- `output_format=png|jpeg` -> `OutputConfig.OutputFormat`
- `images` / `image` -> `FileInfos`，支持 URL、data URL 或裸 Base64，最多 16 张

`size` 支持 `auto` 或 `WIDTHxHEIGHT`。自定义宽高必须是 16 的倍数，最长边不超过 3840，总像素数为 655,360 到 8,294,400。

## 内部任务流程

1. 使用 VOD `CreateAigcImageTask` 提交任务。
2. 使用返回的 `TaskId` 调用 `DescribeTaskDetail`。
3. 轮询到 `AigcImageTask.Status=FINISH`。
4. 将 `AigcImageTask.Output.FileInfos[].FileUrl` 转为 OpenAI Images 响应。

单次请求最长轮询 10 分钟。`response_format=url` 直接返回 VOD 临时 URL；其他值会下载图片并返回 `b64_json`。
