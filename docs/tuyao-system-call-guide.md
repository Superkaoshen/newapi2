# 本系统 Tuyao 异步图片调用文档

本文档描述客户端调用本系统时的格式。Tuyao 上游文档里的直连域名是 `https://api.tuyaoai.com`，客户端接入本系统时应替换为本系统域名。

## 渠道配置

| 配置项 | 值 |
|---|---|
| 渠道名称 | `Tuyao` |
| 渠道类型 | `64` |
| 默认 Base URL | `https://api.tuyaoai.com` |
| 任务平台 | `tuyao` |
| 对外模型名 | `tuyao/nanobananapro` |

注意：

- 调用本系统时模型必须带 `tuyao/` 前缀，例如 `tuyao/nanobananapro`。
- 系统转发给 Tuyao 上游时会去掉 `tuyao/` 前缀，上游收到的模型名是 `nanobananapro`。
- `/v1/async/generations` 仍兼容 Mihuifang；只有模型名以 `tuyao/` 开头时才会路由到 Tuyao 渠道。
- 如果不带 `tuyao/` 前缀，`/v1/async/generations` 默认仍按 Mihuifang 逻辑处理。

## 鉴权

客户端请求本系统时使用本系统发放的 API Key：

```http
Authorization: Bearer sk-xxx
Content-Type: application/json
```

渠道里配置的 Tuyao 上游 Key 只在服务端使用，客户端不需要也不应该直接传上游 Key。

## 接口列表

| 用途 | 方法 | 路径 | 说明 |
|---|---|---|---|
| 提交图片任务 | `POST` | `/v1/async/generations` | 传 `tuyao/` 模型时路由到 Tuyao |
| 推荐查询任务 | `GET` | `/v1/tasks/{requestId}` | 推荐使用 |
| 兼容查询任务 | `GET` | `/v1/async/generations/{requestId}` | 旧版兼容入口 |

## 文生图提交

```bash
curl -X POST "https://your-new-api.example.com/v1/async/generations" \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tuyao/nanobananapro",
    "mode": "text_to_image",
    "prompt": "一只未来感狐狸站在霓虹街头，电影级光影，细节丰富",
    "size": "16x9-4K",
    "n": 1,
    "response_format": "url"
  }'
```

## 图生图提交

```bash
curl -X POST "https://your-new-api.example.com/v1/async/generations" \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tuyao/nanobananapro",
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
| `model` | string | 是 | 必须使用本系统模型名，例如 `tuyao/nanobananapro`。 |
| `prompt` | string | 是 | 图片生成或编辑提示词。 |
| `mode` | string | 否 | `text_to_image` / `image_to_image`。不传时系统会根据是否有参考图自动推导。 |
| `image` | string | 否 | 单张参考图，支持 URL、data URI 或 base64。 |
| `images` | array | 否 | 多张参考图。 |
| `referenceImages` | array | 否 | 兼容参考图字段。 |
| `reference_images` | array | 否 | `referenceImages` 的 snake_case 兼容字段。 |
| `size` | string | 否 | 旧格式如 `16x9-4K`、`1x1`，也可传上游支持的像素尺寸。 |
| `aspect_ratio` | string | 否 | 比例字段，如 `1:1`、`16:9`、`9:16`。 |
| `resolution` | string | 否 | 图片档位，如 `1K`、`2K`、`4K`。 |
| `quality` | string | 否 | 透传字段。 |
| `n` | number | 否 | 输出数量；不传或小于等于 0 时按 `1` 处理。 |
| `response_format` | string | 否 | 常用值为 `url`。 |
| `output_psd` | boolean | 否 | 透传字段。 |

## 提交响应

提交成功时，本系统会直接返回 Tuyao 提交响应。后续查询使用响应里的 `requestId`。

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

说明：

- 本系统会把 `requestId` 保存为任务 ID。
- 旧 Mihuifang 适配器只识别 `task_id` / `id`，Tuyao 返回的是 `requestId`，所以不能复用 Mihuifang 渠道。

## 查询任务

推荐使用：

```bash
curl -X GET "https://your-new-api.example.com/v1/tasks/task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz" \
  -H "Authorization: Bearer sk-xxx"
```

兼容入口：

```bash
curl -X GET "https://your-new-api.example.com/v1/async/generations/task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz" \
  -H "Authorization: Bearer sk-xxx"
```

## 查询响应

查询接口返回本系统统一任务响应，`data` 内是上游任务结果。

### 处理中

```json
{
  "code": "success",
  "data": {
    "requestId": "task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz",
    "modelCode": "nanobananapro",
    "status": "processing",
    "progress": 30
  }
}
```

### 成功

```json
{
  "code": "success",
  "data": {
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
}
```

### 失败

```json
{
  "code": "success",
  "data": {
    "requestId": "task_FqPkaXok2OlUpRxTrIa8qZO5t6Wfi8Rz",
    "modelCode": "nanobananapro",
    "status": "failed",
    "progress": 100,
    "error": {
      "message": "task failed",
      "code": "task_failed"
    }
  }
}
```

## 状态映射

| Tuyao 状态 | 本系统任务状态 |
|---|---|
| `submitted` | `SUBMITTED` |
| `pending` / `queued` / `queue` | `QUEUED` |
| `processing` / `running` / `in_progress` | `IN_PROGRESS` |
| `succeeded` / `completed` / `complete` / `success` | `SUCCESS` |
| `failed` / `failure` / `error` | `FAILURE` |

## 价格配置

Tuyao 计费模型名按本系统公开模型名配置：

```json
{
  "tuyao/nanobananapro": 0.08
}
```

如果配置了渠道模型映射，建议仍优先给公开模型名 `tuyao/nanobananapro` 配价格，避免上游模型名变化影响用户侧计费口径。

## 常见问题

### 为什么之前报 `task id missing in response`

因为 Tuyao 提交响应返回的是：

```json
{
  "requestId": "task_xxx"
}
```

而旧 `mihuifang` provider 只读取：

```json
{
  "task_id": "xxx",
  "id": "xxx"
}
```

字段不一致时，系统拿不到任务 ID，就会返回：

```json
{
  "code": "invalid_response",
  "message": "task id missing in response",
  "data": null
}
```

因此 Tuyao 需要使用独立 `tuyao` 渠道。
