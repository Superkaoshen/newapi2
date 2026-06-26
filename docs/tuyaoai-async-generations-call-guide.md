# TuyaoAI 旧版异步图片接口调用文档

本文档整理旧版兼容入口：

- `POST https://api.tuyaoai.com/v1/async/generations`
- `GET https://api.tuyaoai.com/v1/tasks/{requestId}`
- `GET https://api.tuyaoai.com/v1/async/generations/{requestId}`

其中 `/v1/tasks/{requestId}` 是推荐查询入口，`/v1/async/generations/{requestId}` 是旧版兼容查询入口。

## 鉴权

所有请求都需要携带系统发放的 API Key：

```http
Authorization: Bearer sk-xxx
```

JSON 请求还需要：

```http
Content-Type: application/json
```

## 提交任务

```bash
curl -X POST "https://api.tuyaoai.com/v1/async/generations" \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nanobananapro",
    "mode": "text_to_image",
    "prompt": "一只未来感狐狸站在霓虹街头，电影级光影，细节丰富",
    "size": "16x9-4K",
    "n": 1,
    "response_format": "url"
  }'
```

### 图生图示例

```bash
curl -X POST "https://api.tuyaoai.com/v1/async/generations" \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nanobananapro",
    "mode": "image_to_image",
    "prompt": "把图片改成水彩插画风格",
    "images": [
      "https://example.com/input.png"
    ],
    "size": "16x9-4K",
    "n": 1,
    "response_format": "url"
  }'
```

## 请求字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `model` | string | 是 | 模型名。可传公开模型名，也可通过渠道模型映射映射到上游模型。 |
| `prompt` | string | 是 | 图片生成或编辑提示词。 |
| `mode` | string | 否 | `text_to_image` / `image_to_image`；兼容 `edit`、`image_edit`、`img2img`。 |
| `image` | string | 否 | 单张参考图，支持 URL、data URI、纯 base64。 |
| `images` | array | 否 | 多张参考图，支持 URL、data URI、纯 base64。 |
| `referenceImages` | array | 否 | 兼容参考图字段。 |
| `reference_images` | array | 否 | `referenceImages` 的 snake_case 兼容字段。 |
| `size` | string | 否 | 旧格式如 `16x9-4K`、`1x1`，也支持官方像素如 `5504x3072`。 |
| `aspect_ratio` | string | 否 | 比例，如 `1:1`、`16:9`、`9:16`、`4:3`、`3:4`。 |
| `resolution` | string | 否 | 图片档位：`1K` / `2K` / `4K`。 |
| `quality` | string | 否 | `gpt-image-2` 支持 `low` / `medium` / `high`。Nano 系列会按尺寸档位自动推导。 |
| `n` | number | 否 | 输出数量，建议传 `1`。 |
| `response_format` | string | 否 | `url` 或 `b64_json`。 |
| `output_psd` | boolean | 否 | 仅 `gpt-image-2` 使用，传 `true` 时请求 PSD 输出。 |

## 尺寸兼容

旧版 `size` 会在系统内部映射成上游需要的官方像素尺寸。

示例：

| 旧格式 | 等价含义 |
|---|---|
| `1x1` | `1:1`，默认 `1K` |
| `16x9-1K` | `16:9`，`1K` |
| `16x9-2K` | `16:9`，`2K` |
| `16x9-4K` | `16:9`，`4K` |
| `9x16-4K` | `9:16`，`4K` |

也可以拆成：

```json
{
  "aspect_ratio": "16:9",
  "resolution": "4K"
}
```

或者直接传官方像素尺寸：

```json
{
  "size": "5504x3072"
}
```

## 提交响应

```json
{
  "taskOrderId": 2041888888888888888,
  "requestId": "task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz",
  "modelCode": "nanobananapro",
  "status": "submitted",
  "billingStatus": "pending",
  "progress": 20
}
```

客户端后续查询必须使用响应里的 `requestId`，也就是本系统公开的 `task_xxx`。不要使用上游原始 `requestId` 或 `taskOrderId` 查询本系统。

## 查询任务

推荐查询：

```bash
curl -X GET "https://api.tuyaoai.com/v1/tasks/task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz" \
  -H "Authorization: Bearer sk-xxx"
```

旧版兼容查询：

```bash
curl -X GET "https://api.tuyaoai.com/v1/async/generations/task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz" \
  -H "Authorization: Bearer sk-xxx"
```

## 查询响应

### 排队或处理中

```json
{
  "requestId": "task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz",
  "modelCode": "nanobananapro",
  "status": "processing",
  "progress": 30
}
```

常见状态：

| 状态 | 说明 |
|---|---|
| `pending` | 等待处理 |
| `submitted` | 已提交 |
| `processing` | 处理中 |
| `succeeded` | 成功 |
| `failed` | 失败 |

### 成功

```json
{
  "requestId": "task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz",
  "modelCode": "nanobananapro",
  "status": "succeeded",
  "progress": 100,
  "resultCount": 1,
  "result": {
    "image_url": "https://cdn.example.com/result.png",
    "url": "https://cdn.example.com/result.png",
    "items": [
      {
        "url": "https://cdn.example.com/result.png",
        "type": "image"
      }
    ]
  },
  "url": "https://cdn.example.com/result.png"
}
```

### 失败

```json
{
  "requestId": "task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz",
  "modelCode": "nanobananapro",
  "status": "failed",
  "progress": 100,
  "error": {
    "message": "task failed",
    "code": "task_failed"
  }
}
```

## 注意事项

- `/v1/async/generations` 是旧版兼容提交入口，图片新接入优先使用 `/v1/images/generations` 和 `/v1/images/edits`。
- 查询任务时必须使用本系统返回的 `requestId`，格式通常是 `task_xxx`。
- `/v1/tasks/{requestId}` 和 `/v1/async/generations/{requestId}` 查询的是同一套任务数据。
- 对外响应里的 `modelCode` 使用用户请求的公开模型名，不暴露上游 `modelName`。
- 成功结果里的图片 URL 是系统转存后的 URL；如果转存失败，任务会返回失败，不直接暴露上游原始 URL。
