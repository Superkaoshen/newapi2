# Gemini 图片旧异步入口多渠道配置文档

本文档说明如何让用户继续调用旧异步接口：

```http
POST /v1/async/generations
GET /v1/tasks/{requestId}
```

同时在后台把同一个公开模型分配到多个不同协议的图片渠道中，实现提交阶段的负载和失败切换。

## 对外调用格式

客户端请求保持旧异步格式，不需要使用 Gemini 官方 `generateContent` 请求体：

```bash
curl -X POST "https://api.tuyaoai.com/v1/async/generations" \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3-pro-image-preview",
    "mode": "image_to_image",
    "prompt": "让这只猫戴上宇航员头盔",
    "images": ["https://example.com/cat.png"],
    "size": "16x9-1K",
    "n": 1,
    "response_format": "url"
  }'
```

系统内部会根据命中的渠道类型转换请求格式。

## 渠道 A：Gemini 官方兼容图片渠道

用于接入兼容 Google Gemini 官方接口的图片上游。

| 配置项 | 值 |
|---|---|
| 渠道类型 | `Gemini` |
| Base URL | `https://api.nanobananai.com` |
| 模型 | `gemini-3-pro-image-preview` |
| 模型映射 | 通常不需要 |

系统会把旧异步请求转换成：

```http
POST /v1beta/models/gemini-3-pro-image-preview:generateContent
```

转换规则：

| 旧异步字段 | Gemini 官方字段 |
|---|---|
| `prompt` | `contents[].parts[].text` |
| `image` / `images` / `referenceImages` / `reference_images` | `contents[].parts[].inlineData` 或 `fileData` |
| `size` / `aspect_ratio` | `generationConfig.imageConfig.aspectRatio` |
| `size` / `resolution` | `generationConfig.imageConfig.imageSize` |
| `n` | `generationConfig.candidateCount` |

Gemini 图片上游返回图片后，系统会转存 OSS，并把任务直接标记为 `succeeded`。

## 渠道 B：AIAPIPro 图片渠道

用于接入 AIAPIPro 图片接口。

| 配置项 | 值 |
|---|---|
| 渠道类型 | `Mihuifang` |
| Base URL | `https://aiapipro.vip` 或实际上游地址 |
| 模型 | `gemini-3-pro-image-preview` |
| 模型映射 | `{"gemini-3-pro-image-preview":"nanobananapro"}` |
| 其他设置 | 空或 `{"image_task_protocol":"aiapipro"}` |

## 渠道 C：兼容图片编辑协议渠道

用于接入 `POST /v1/images/edits`、`GET /v1/status/{task_id}` 这类图片上游。

| 配置项 | 值 |
|---|---|
| 渠道类型 | `Mihuifang` |
| Base URL | 实际上游地址 |
| 模型 | `gemini-3-pro-image-preview` |
| 模型映射 | `{"gemini-3-pro-image-preview":"banana-pro"}` |
| 其他设置 | `{"image_task_protocol":"imageone"}` |

## 负载与失败切换

把多个渠道都配置为同一个公开模型名，例如：

```text
gemini-3-pro-image-preview
```

系统会按现有能力表的 `priority` 和 `weight` 选择渠道：

- 相同 `priority`：按 `weight` 随机负载。
- 不同 `priority`：优先使用高优先级渠道，提交失败后切到下一优先级。
- 只有提交阶段失败才会切换渠道；任务提交成功后会绑定该渠道，后续查询不会跨渠道迁移。

## 价格配置

建议为公开模型配置基础价和分辨率档位价：

```json
{
  "gemini-3-pro-image-preview": 0.08,
  "gemini-3-pro-image-preview@1k": 0.08,
  "gemini-3-pro-image-preview@2k": 0.12,
  "gemini-3-pro-image-preview@4k": 0.20
}
```

最终费用会按 `n` 生成数量放大。

## 注意事项

- 本方案不要求客户端传 Gemini 官方 `contents` 请求体。
- 本方案暂不在旧异步格式中暴露 `tools.google_search`；后续可通过 `metadata` 扩展。
- Gemini 图片结果必须成功转存 OSS 后才会返回给用户；如果 OSS 未配置或转存失败，任务会失败，不会泄露上游原始 base64。
