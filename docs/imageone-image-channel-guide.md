# ImageOne 图片渠道配置指南

本文档说明如何把兼容图片编辑协议的上游配置到本系统的异步图片渠道中。代码内使用中性协议名 `imageone`，渠道名称和 Base URL 由管理员在后台配置。

## 适用场景

- 上游提交端点：`POST /v1/images/edits`
- 上游查询端点：`GET /v1/status/{task_id}`
- 提交格式：`multipart/form-data`
- 支持 `image` 单文件、`images` 多文件、`reference_image_urls` URL/UUID 混传。
- 上游结果支持 `{"images":[{"url":"..."}]}` 或 `{"images":[{"b64_json":"..."}]}`。

## 渠道配置

在后台新增或编辑渠道：

| 配置项 | 值 |
| --- | --- |
| 渠道类型 | 选择现有异步图片渠道 |
| Base URL | 填上游域名，例如 `https://image.example.com` |
| Key | 上游 API Key |
| 模型 | 填公开模型名或上游模型名 |
| 其他设置 | `{"image_task_protocol":"imageone"}` |

`image_task_protocol` 说明：

- 不填或填 `aiapipro`：保持原 AIAPIPro 协议，提交到 `/v1/images/generations` 或 `/v1/images/edits`，查询 `/v1/tasks/{requestId}`。
- 填 `imageone`：提交到 `/v1/images/edits`，查询 `/v1/status/{task_id}`。

## 模型映射

如果希望用户继续调用系统公开模型名，例如 `gemini-3-pro-image`，但上游实际模型是 `banana-pro`，配置模型映射：

```json
{
  "gemini-3-pro-image": "banana-pro"
}
```

这样用户请求：

```json
{
  "model": "gemini-3-pro-image",
  "prompt": "把图片改成水彩插画风格",
  "size": "16x9-4K"
}
```

转发到该渠道时，上游收到的 `model` 会是 `banana-pro`。

## Multipart 调用示例

```bash
curl -X POST "https://your-new-api-domain/v1/images/edits" \
  -H "Authorization: Bearer sk-your-new-api-key" \
  -F "prompt=a cat wearing sunglasses" \
  -F "model=gemini-3-pro-image" \
  -F "aspect_ratio=1:1" \
  -F "response_format=url" \
  -F "image=@reference.jpg"
```

多图参考：

```bash
curl -X POST "https://your-new-api-domain/v1/images/edits" \
  -H "Authorization: Bearer sk-your-new-api-key" \
  -F "prompt=a cat wearing sunglasses" \
  -F "model=gemini-3-pro-image" \
  -F "size=16x9-4K" \
  -F "response_format=url" \
  -F "images=@reference-1.jpg" \
  -F "images=@reference-2.jpg"
```

## 字段映射

| 用户字段 | ImageOne 上游字段 | 说明 |
| --- | --- | --- |
| `model` | `model` | 先经过渠道模型映射，再传给上游 |
| `prompt` | `prompt` | 原样透传 |
| `image` 文件 | `image` 文件 | 单张参考图 |
| `images` 文件 | `images` 文件 | 多张参考图，最多 5 张 |
| `reference_image_urls` | `reference_image_urls` | URL 或上游上传 UUID |
| `image` / `images` JSON URL | `reference_image_urls` | JSON 调用时自动转换 |
| `size=16x9-4K` | `aspect_ratio=16:9`, `resolution=4K` | 旧尺寸写法会自动拆分 |
| `aspect_ratio` | `aspect_ratio` | 优先使用显式传入值 |
| `response_format=b64_json` | `response_format=base64` | 兼容 OpenAI 写法 |
| `response_format` 为空 | `response_format=url` | 系统默认推荐 URL，便于结果入 OSS |

## 任务查询

提交成功后，本系统仍返回公开 `requestId`，不会把上游 `task_id` 暴露给用户。用户按本系统统一接口查询：

```bash
curl "https://your-new-api-domain/v1/tasks/{requestId}" \
  -H "Authorization: Bearer sk-your-new-api-key"
```

系统内部会按提交时记录的 `image_task_protocol` 去请求上游：

```text
GET {Base URL}/v1/status/{task_id}
```

上游状态映射：

| 上游状态 | 系统状态 |
| --- | --- |
| `pending` | `queued` |
| `processing` | `in_progress` |
| `completed` | `succeeded` |
| `failed` | `failed` |

## 多渠道轮询/故障切换

同一个公开模型可以配置多个渠道：

1. 渠道 A：默认 AIAPIPro 协议，模型映射到 `nanobananapro`。
2. 渠道 B：`image_task_protocol=imageone`，模型映射到 `banana-pro`。

两个渠道都把 `gemini-3-pro-image` 加入模型列表。提交时如果渠道 A 失败，系统会按现有重试/渠道选择逻辑切到下一个可用渠道；每个渠道会使用自己的协议构造请求，不要求两个上游调用格式一致。

注意：已经提交成功的任务会记录提交时使用的协议。后续轮询不会因为管理员修改渠道配置而切换查询端点。
