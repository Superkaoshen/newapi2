# 异步图片生成渠道调用文档

本文档描述本系统适配后的异步图片生成接口。对外只暴露本系统地址、任务 ID 与统一状态，不暴露上游供应商信息。

## 渠道添加规则

1. 在渠道管理中新增一个可自定义基础地址的渠道。
2. 基础地址填写上游节点根地址，不要带 `/v1/api/generate` 或 `/v1/api/result`。
3. 密钥填写上游 API Key，系统会以 `Authorization: Bearer <key>` 转发。
4. 模型列表至少配置需要开放的模型名，例如：
   - `nano-banana`
   - `nano-banana-fast`
   - `nano-banana-2`
   - `nano-banana-2-cl`
   - `nano-banana-2-4k-cl`
   - `nano-banana-pro`
   - `nano-banana-pro-cl`
   - `nano-banana-pro-vip`
   - `nano-banana-pro-4k-vip`
   - `gpt-image-2`
   - `gpt-image-2-vip`
5. 前端任务日志中平台展示为“异步渠道”，动作展示为异步图片生成，并在成功后支持图片预览。

> 注意：该渠道复用本系统模型分发能力。调用 `/v1/api/generate` 时按请求体中的 `model` 选择渠道，因此模型名必须加入渠道模型列表和令牌可用模型范围。

## 提交任务

`POST /v1/api/generate`

### Header

```http
Authorization: Bearer <系统令牌>
Content-Type: application/json
```

### Body

```json
{
  "model": "nano-banana-2",
  "prompt": "提取图片中的印花图案，输出干净的可复用图案素材",
  "images": ["https://example.com/source.jpg"],
  "aspectRatio": "1:1",
  "imageSize": "1K",
  "replyType": "async"
}
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 是 | 支持 `nano-banana*` 与 `gpt-image-2*` 系列模型 |
| `prompt` | 是 | 生成或提取提示词 |
| `images` | 否 | 参考图数组，支持 URL 或 base64 |
| `aspectRatio` | 否 | 比例或像素值；`gpt-image-2-vip` 建议传像素值 |
| `imageSize` | 否 | `nano-banana` 系列可传 `1K`、`2K`、`4K` |
| `replyType` | 否 | 客户端可传 `json`、`stream`、`async`；系统提交任务时统一按异步任务处理 |

### Response

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "running"
}
```

返回的 `id` 是本系统公开任务 ID，用于查询结果和任务日志。

## 查询任务

`GET /v1/api/result?id=<task_id>`

### Header

```http
Authorization: Bearer <系统令牌>
```

### 成功中

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "running",
  "progress": 45
}
```

### 生成成功

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "succeeded",
  "progress": 100,
  "results": [
    {
      "url": "https://example.com/generated.png"
    }
  ]
}
```

### 生成失败

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "failed",
  "progress": 100,
  "error": "generate failed"
}
```

状态说明：

| 状态 | 说明 |
| --- | --- |
| `running` | 任务进行中 |
| `succeeded` | 任务成功 |
| `failed` | 任务失败 |
| `violation` | 上游判定违规时会按失败写入任务日志 |

## 调用示例

```bash
curl -X POST "https://your-domain.com/v1/api/generate" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "从参考图中提取连续印花图案，背景透明，保留主要色块",
    "images": ["https://example.com/fabric.jpg"],
    "aspectRatio": "1024x1024",
    "replyType": "async"
  }'
```

```bash
curl "https://your-domain.com/v1/api/result?id=task_xxxxxxxxxxxxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

