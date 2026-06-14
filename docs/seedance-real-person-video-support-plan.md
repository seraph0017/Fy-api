# Seedance 2.0 真人视频支持方案

> 日期：2026-06-14
> 参考项目：`~/go/src/tracenex/DJLine`
> 适用仓库：TraceNex / Fy-api

## 1. DJLine 当前做法

DJLine 不是直接把真人公网图片 URL 提交给 Seedance 2.0，而是在视频生成前引入“可信素材准备”阶段：

1. 用户在视频候选里绑定具体候选图 ID，后端读取这些候选图的公网 `image_url`。
2. 后端调用火山 Ark Asset Service `CreateAsset`。
3. 候选图进入 `provider_asset_status` 状态机：`unprepared -> creating/processing -> active/failed`。
4. 远端审核通过后保存 `provider_asset_id` 和 `provider_asset_uri`，URI 形如 `asset://<provider_asset_id>`。
5. 投递 Seedance worker 时，最终使用 `content[].image_url.url = asset://<provider_asset_id>`。

关键约束：

- 真实 Seedance 通道不允许 `asset://mock_...`。
- 不用 COS/公网 URL 代替 `asset://...` 投递给 direct Seedance 真人/可信素材路径。
- `public_url` 只用于审计或其他非 direct Seedance 路由，不作为 direct Seedance 主输入。
- `InputImageSensitiveContentDetected.PrivacyInformation` 应视为上游拒绝未受信真人/隐私图片输入。

## 2. Fy-api 第一阶段实现

Fy-api 采用“客户 API 不变，内部异步准备素材”的方案：

1. 客户仍然调用 `/v1/videos`，传普通 `image`、`images[]` 或 `metadata.content[].image_url.url`。
2. 命中 Doubao/VolcEngine Seedance 2.0 且存在 HTTP/HTTPS 图片参考时，提交阶段不立刻投递 Seedance。
3. 系统按现有异步任务模型创建本地 task，并在 `task.private_data.seedance_asset_prepare` 保存原始请求快照和参考图列表。
4. 轮询器先调用 Ark Asset Service `CreateAsset`，再用 `GetAsset` 同步审核状态。
5. 全部参考图 `active` 后，内部把原请求中的公网图 URL 改写为 `asset://<provider_asset_id>`，再提交 Seedance 原生视频任务。
6. 拿到真实 Seedance task id 后，任务回到既有轮询流程。
7. 素材创建/审核/二次提交失败时，任务进入 `FAILURE`，复用现有异步失败退款逻辑。

对外返回仍是 OpenAI-like video task；客户不需要理解 Ark asset id，也不需要新增素材 API 才能用真人图。

## 3. 计费策略

当前实现沿用 fy-api 现有异步视频任务的预扣模式：

- `/v1/videos` 提交成功时先按模型价格预扣，防止无余额用户占用素材审核和视频队列资源。
- 如果 Ark asset 创建失败、审核失败或 Seedance 二次提交失败，任务失败并触发 `RefundTaskQuota`，客户最终不付费。
- 如果素材通过并最终视频成功，按现有任务完成结算逻辑处理。

产品口径建议描述为“提交时预授权，失败自动退款”，不要承诺素材审核失败时从未产生任何临时额度冻结。

火山侧是否对 `CreateAsset`/审核失败收费，需要以火山合同和账单为准；fy-api 第一阶段不把素材准备费用单独转嫁给客户，统一并入成功视频任务成本。若后续火山对失败审核产生显著成本，应补风控策略：频率限制、最小余额门槛、失败率熔断、素材准备成本报表。

## 4. 渠道配置与页面入口

Ark Asset Service 凭证跟具体 Seedance 渠道绑定，放在渠道 `settings` JSON 中，不使用全局环境变量，也不新增 `channels` 表字段。渠道类型仍使用现有 type `45`（字节火山方舟/豆包通用）：

- `channel.key`：继续作为 Seedream / Seedance 主视频接口的 API key；豆包语音的 `APPID|AccessToken` 特例不变。
- `channel.settings`：保存 Ark Asset Service 的 AK/SK/GroupId 等附加配置。

最小配置：

```json
{
  "seedance_asset_access_key": "火山 AK",
  "seedance_asset_secret_key": "火山 SK",
  "seedance_asset_group_id": "Ark Asset GroupId",
  "seedance_asset_project_name": "default",
  "seedance_asset_region": "cn-beijing",
  "seedance_asset_timeout_seconds": 20
}
```

可选字段：

- `seedance_asset_endpoint`
- `seedance_asset_project_name`，默认 `default`
- `seedance_asset_region`，默认 `cn-beijing`
- `seedance_asset_timeout_seconds`，默认 `20`

页面入口在 `web/classic` 渠道编辑弹窗中：当渠道类型为 `45` 时，在“额外设置”下显示 `Seedance 真人素材库` 区块，写入上述 `settings` JSON key。普通文本、图片和视频生成继续使用上方“渠道密钥”，只有命中 Seedance 2.0 + HTTP/HTTPS 图片参考的真人素材准备流程时才读取这些配置。

未配置 AK/SK 时，SD2.0 真人图任务会进入素材准备失败并退款；老渠道没有这些 key 时，非素材准备路径不受影响。

## 5. 测试策略

稳定自动化回归：

- 使用非真人产品图、风格图、自制视频。
- 不使用公众人物、名人、来源不稳定的真人图作为通过用例。

真人能力专项：

- 使用用户授权或平台认可素材。
- 通过 `/v1/videos` 传普通图片 URL，验证内部生成 `asset://...` 后再投递 Seedance。
- 覆盖素材审核失败：任务失败、失败原因可见、额度退款。
- 覆盖 `asset://mock_...`：真实通道不得把 mock asset 投递给上游。

## 6. 后续增强

- 增加用户可见的素材准备状态摘要，但避免长期展示敏感原始真人 URL。
- 增加 provider asset 复用表，避免同一用户同一素材重复审核。
- 增加管理员/调试用 provider asset 查询 API。
- 增加素材准备成本、失败率和退款报表。
- 将 `InputImageSensitiveContentDetected` 映射成更清晰的客户错误文案。
