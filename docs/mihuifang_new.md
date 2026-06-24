# AIAPIPro 异步图片调用文档

本文档描述客户端调用本系统的异步图片接口方式。客户端只需要调用本系统域名，不需要知道上游地址或上游 Key。

AIAPIPro 官方模型字段、尺寸表、参考图限制和上游价格以 [AIAPIPro 图片模型官方参考文档](./aiapipro-image-models-reference.md) 为准。后续修改 `relay/channel/task/mihuifang` 适配器前，必须先核对该参考文档。

示例中的 `https://你的系统域名` 请替换为实际部署地址，`YOUR_API_KEY` 请替换为本系统发放给用户的 API Key。

## 1. 接口总览

| 用途 | 方法 | 路径 | 说明 |
|---|---|---|---|
| 文生图 | `POST` | `/v1/images/generations` | 推荐的图片生成入口。 |
| 图生图/图片编辑 | `POST` | `/v1/images/edits` | 推荐的图片编辑入口，支持 JSON 和 multipart。 |
| 查询任务 | `GET` | `/v1/tasks/{task_id}` | 推荐的任务查询入口。 |
| 兼容查询 | `GET` | `/v1/async/generations/{task_id}` | 旧版兼容查询入口。 |
| 兼容提交 | `POST` | `/v1/async/generations` | 旧版兼容提交入口，仍可用。 |

通用请求头：

```http
Authorization: Bearer YOUR_API_KEY
Content-Type: application/json
```

multipart 图片编辑时：

```http
Authorization: Bearer YOUR_API_KEY
Content-Type: multipart/form-data
```

## 2. 当前内置模型

| 公开模型名 | 上游模型码 | 适用场景 | 计费维度 |
|---|---|---|---|
| `gpt-image-2` | `gpt-image-2` | 高质量图片生成、图片编辑、PSD 输出 | `quality`、`output_psd`、`n` |
| `nanobanana` | `nanobanana` | 快速通用文生图/图生图 | 清晰度档位、`n` |
| `nanobanana2` | `nanobanana2` | 更强的快速图片生成 | 清晰度档位、`n` |
| `nanobananapro` | `nanobananapro` | 高质量复杂提示词图片生成 | 清晰度档位、`n` |

兼容旧别名：

| 旧别名 | 自动映射到 |
|---|---|
| `nano-banana` | `nanobanana` |
| `nano-banana2` | `nanobanana2` |
| `nano-banana-pro` | `nanobananapro` |

如果需要暴露 `gemini-2.5-flash-image`、`gemini-3-pro-image` 等自定义名称，请在渠道编辑里的“模型映射”配置公开名到上游模型码。详见“模型映射”章节。

当前 aiapipro/mihuifang 图片适配器未内置 `vectorizer`。如果要对外提供 `vectorizer`，需要另行接入对应渠道；仅在模型映射里把它映射到上述图片模型时，它才会按被映射的图片模型工作。

## 3. 通用请求字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `model` | string | 是 | 公开模型名，或渠道模型映射里的 source 名。 |
| `prompt` | string | 是 | 图片生成/编辑提示词。 |
| `image` | string 或 string[] | 否 | 单图或多图输入。传数组时系统会兼容处理。 |
| `images` | string[] | 否 | 多图输入。 |
| `referenceImages` | string[] | 否 | 参考图输入。 |
| `reference_images` | string[] | 否 | `referenceImages` 的 snake_case 兼容字段。 |
| `mask` | object/string | 否 | 图片编辑 mask，按上游可接受格式透传。 |
| `size` | string | 否 | 官方 OpenAI 兼容像素尺寸。Nano 系列从官方尺寸表选择，例如 `2048x2048`、`1536x2752`；`gpt-image-2` 直接传自定义 `WIDTHxHEIGHT`。 |
| `quality` | string | 否 | Nano 系列由尺寸档位推导为 `standard`/`hd`；`gpt-image-2` 使用 `low`、`medium`、`high`，不传时按 `low` 档计费。 |
| `output_psd` | bool/string | 否 | 仅 `gpt-image-2` 使用。为 `true` 时请求 PSD 输出并按 PSD 档计费。 |
| `response_format` | string | 否 | 按上游支持情况透传。 |
| `n` | int | 否 | 生成数量；不传时按 1 计费。 |

清晰度档位识别规则：

| 官方 `size` 示例 | 计费档位 |
|---|---|
| `1024x1024`、`1584x672`、空值 | `1k` |
| `2048x2048`、`1536x2752` | `2k` |
| `4096x4096`、`3072x5504` | `4k` |

建议直接传官方像素 `size`。历史的“比例 + 档位”组合写法、`aspect_ratio + resolution` 写法仅作为兼容输入，系统会在发送上游前转换为官方像素尺寸。

## 4. 创建任务

### 4.1 文生图

```bash
curl -X POST "https://你的系统域名/v1/images/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nanobanana",
    "prompt": "一张干净白底的高端香水产品图，柔光摄影，商业广告风格",
    "size": "2048x2048",
    "n": 1
  }'
```

成功响应：

```json
{
  "requestId": "task_xxx",
  "modelCode": "nanobanana",
  "status": "submitted",
  "progress": 0
}
```

### 4.2 图生图 / 图片编辑

JSON 请求：

```bash
curl -X POST "https://你的系统域名/v1/images/edits" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nanobananapro",
    "prompt": "保留主体姿态，把背景改成高级摄影棚，商业海报风格",
    "image": "https://example.com/input.png",
    "size": "5504x3072",
    "n": 1
  }'
```

multipart 请求：

```bash
curl -X POST "https://你的系统域名/v1/images/edits" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -F "model=nanobanana" \
  -F "prompt=把图片改成日系动画电影海报风格" \
  -F "size=1536x2752" \
  -F "image=@/path/to/input.png"
```

## 5. 查询任务

推荐使用：

```bash
curl -X GET "https://你的系统域名/v1/tasks/task_xxx" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

兼容旧接口：

```bash
curl -X GET "https://你的系统域名/v1/async/generations/task_xxx" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

处理中响应：

```json
{
  "requestId": "task_xxx",
  "modelCode": "nanobanana",
  "status": "processing",
  "progress": 30
}
```

成功响应：

```json
{
  "requestId": "task_xxx",
  "modelCode": "nanobanana",
  "status": "succeeded",
  "progress": 100,
  "result": {
    "image_url": "https://your-oss-domain/async-images/xxx.png",
    "url": "https://your-oss-domain/async-images/xxx.png",
    "items": [
      {
        "url": "https://your-oss-domain/async-images/xxx.png",
        "type": "image"
      }
    ]
  },
  "url": "https://your-oss-domain/async-images/xxx.png"
}
```

失败响应：

```json
{
  "requestId": "task_xxx",
  "modelCode": "nanobanana",
  "status": "failed",
  "progress": 100,
  "error": {
    "message": "task failed",
    "code": "task_failed"
  }
}
```

结果文件说明：

- 成功结果里的 URL 是本系统转存后的 OSS/CDN URL。
- 如果结果文件转存 OSS 失败，任务会失败，不会把上游原始 URL 返回给用户。
- 响应里的 `modelCode` 始终使用用户请求的公开模型名，不返回上游 `modelName`。

## 6. 模型映射

渠道编辑页面支持配置“模型映射”，格式是 JSON 对象：

```json
{
  "公开模型名": "上游模型码"
}
```

推荐配置示例：

```json
{
  "gemini-2.5-flash-image": "nanobanana",
  "gemini-3.1-flash-image": "nanobanana2",
  "gemini-3-pro-image": "nanobananapro",
  "gemini-3-pro-image-preview": "gpt-image-2"
}
```

配置后，用户可以请求公开模型名：

```json
{
  "model": "gemini-3-pro-image",
  "prompt": "一张高端电动汽车发布会主视觉海报",
  "size": "5504x3072"
}
```

系统会：

1. 使用 `gemini-3-pro-image` 作为用户可见模型名和日志模型名。
2. 发送给上游时使用 `nanobananapro`。
3. 如果 `gemini-3-pro-image` 没有单独配置价格，则按 `nanobananapro` 的默认价格计费。
4. 查询响应中的 `modelCode` 返回 `gemini-3-pro-image`，不会返回上游模型名。

注意：渠道能力表会自动包含模型映射的 source 名。保存渠道或批量编辑 tag 后，公开模型名可以正常参与渠道选择。

## 7. 每个模型的调用示例

### 7.1 gpt-image-2

适用：高质量图片生成、图片编辑、需要 `quality` 或 `output_psd` 的场景。

文生图：

```bash
curl -X POST "https://你的系统域名/v1/images/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "一张高端腕表产品海报，黑色背景，金属质感，商业摄影",
    "size": "1024x1024",
    "quality": "high",
    "n": 1
  }'
```

图片编辑并输出 PSD：

```bash
curl -X POST "https://你的系统域名/v1/images/edits" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "保留产品主体，把背景换成高级摄影棚，输出可编辑 PSD",
    "image": "https://example.com/product.png",
    "quality": "high",
    "output_psd": true,
    "n": 1
  }'
```

价格：

| 价格 key | 价格 |
|---|---:|
| `gpt-image-2` | 0.10 |
| `gpt-image-2@low` | 0.10 |
| `gpt-image-2@medium` | 0.11 |
| `gpt-image-2@high` | 0.15 |
| `gpt-image-2@low@psd` | 0.16 |
| `gpt-image-2@medium@psd` | 0.17 |
| `gpt-image-2@high@psd` | 0.20 |
| `gpt-image-2@psd` | 0.30 |

计费示例：`quality=high`、`output_psd=true`、`n=2` 命中 `gpt-image-2@high@psd`，费用为 `0.20 * 2 = 0.40`。

### 7.2 nanobanana

适用：快速文生图、轻量图生图、社媒图、头像、海报草图。

```bash
curl -X POST "https://你的系统域名/v1/images/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nanobanana",
    "prompt": "一张赛博朋克城市夜景插画，霓虹灯，高细节",
    "size": "2752x1536",
    "n": 1
  }'
```

价格：

| 价格 key | 价格 |
|---|---:|
| `nanobanana` | 0.04 |
| `nanobanana@1k` | 0.04 |
| `nanobanana@2k` | 0.08 |
| `nanobanana@4k` | 0.12 |

计费示例：`size=5504x3072`、`n=3` 命中 `nanobanana@4k`，费用为 `0.12 * 3 = 0.36`。

### 7.3 nanobanana2

适用：比 `nanobanana` 更强的快速图片生成、复杂一点的参考图重绘。

```bash
curl -X POST "https://你的系统域名/v1/images/edits" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nanobanana2",
    "prompt": "把参考图转成日系动画电影海报风格，保留人物姿态和构图",
    "images": [
      "https://example.com/input.png"
    ],
    "size": "3072x5504",
    "n": 1
  }'
```

价格：

| 价格 key | 价格 |
|---|---:|
| `nanobanana2` | 0.06 |
| `nanobanana2@1k` | 0.06 |
| `nanobanana2@2k` | 0.10 |
| `nanobanana2@4k` | 0.14 |

计费示例：`size=1536x2752`、`n=2` 命中 `nanobanana2@2k`，费用为 `0.10 * 2 = 0.20`。

### 7.4 nanobananapro

适用：高质量复杂提示词图片生成、商业海报、产品图、角色设定图。

```bash
curl -X POST "https://你的系统域名/v1/images/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nanobananapro",
    "prompt": "一张高端电动汽车发布会主视觉海报，黑色车身，未来科技感，电影级灯光",
    "size": "5504x3072",
    "n": 1
  }'
```

价格：

| 价格 key | 价格 |
|---|---:|
| `nanobananapro` | 0.08 |
| `nanobananapro@1k` | 0.08 |
| `nanobananapro@2k` | 0.12 |
| `nanobananapro@4k` | 0.20 |

计费示例：`size=4096x4096`、`n=2` 命中 `nanobananapro@4k`，费用为 `0.20 * 2 = 0.40`。

### 7.5 推荐公开别名模型

如果管理员按本文推荐配置了模型映射，用户也可以直接调用 Gemini 风格公开模型名：

| 公开模型名 | 映射到 | 调用说明 |
|---|---|---|
| `gemini-2.5-flash-image` | `nanobanana` | 用法和 `nanobanana` 一致。 |
| `gemini-3.1-flash-image` | `nanobanana2` | 用法和 `nanobanana2` 一致。 |
| `gemini-3-pro-image` | `nanobananapro` | 用法和 `nanobananapro` 一致。 |
| `gemini-3-pro-image-preview` | `gpt-image-2` | 用法和 `gpt-image-2` 一致，支持 `quality` 和 `output_psd`。 |

`gemini-2.5-flash-image` 示例：

```bash
curl -X POST "https://你的系统域名/v1/images/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "prompt": "一张轻量级社媒海报，明亮色彩，扁平插画风格",
    "size": "2048x2048",
    "n": 1
  }'
```

`gemini-3.1-flash-image` 示例：

```bash
curl -X POST "https://你的系统域名/v1/images/edits" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.1-flash-image",
    "prompt": "把参考图改成电影海报风格，增强光影和质感",
    "image": "https://example.com/input.png",
    "size": "3072x5504",
    "n": 1
  }'
```

`gemini-3-pro-image` 示例：

```bash
curl -X POST "https://你的系统域名/v1/images/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3-pro-image",
    "prompt": "一张奢侈品发布会主视觉，高级摄影棚，复杂布光，商业广告级质感",
    "size": "5504x3072",
    "n": 1
  }'
```

`gemini-3-pro-image-preview` 示例：

```bash
curl -X POST "https://你的系统域名/v1/images/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3-pro-image-preview",
    "prompt": "一张高端产品海报，白色背景，商业摄影，输出高质量版本",
    "quality": "high",
    "output_psd": false,
    "n": 1
  }'
```

## 8. 错误码和排查

| HTTP 状态 | code | 常见原因 | 处理方式 |
|---:|---|---|---|
| 400 | `missing_model` | 未传 `model` | 请求体补充 `model`。 |
| 400 | `invalid_request` | 未传 `prompt` 或 JSON 格式错误 | 检查请求体。 |
| 400 | `model_mapping_failed` | 渠道模型映射 JSON 有误或存在循环 | 检查渠道编辑里的模型映射。 |
| 400 | `model_price_error` | 模型价格或分层价格缺失 | 配置模型价格，或确认映射后的上游模型有默认价格。 |
| 500 | `save_result_file_failed` | 结果转存 OSS 失败 | 检查 OSS 配置、网络、文件大小限制。 |

典型错误响应：

```json
{
  "error": {
    "message": "model price is required for public-model@4k",
    "type": "new_api_error",
    "code": "model_price_error"
  }
}
```

## 9. 管理员配置建议

### 9.1 渠道模型列表

渠道的 `models` 字段建议至少包含上游模型码：

```text
gpt-image-2,nanobanana,nanobanana2,nanobananapro
```

如果使用模型映射，系统会自动把映射 source 加入渠道能力表。历史已保存的渠道如果没有重新保存过，建议重新保存渠道或执行一次能力修复。

### 9.2 模型映射示例

```json
{
  "gemini-2.5-flash-image": "nanobanana",
  "gemini-3.1-flash-image": "nanobanana2",
  "gemini-3-pro-image": "nanobananapro",
  "gemini-3-pro-image-preview": "gpt-image-2"
}
```

### 9.3 自定义价格

如果要覆盖默认价格，可以在后台模型价格中配置：

```json
{
  "gemini-3-pro-image": 0.1,
  "gemini-3-pro-image@1k": 0.1,
  "gemini-3-pro-image@2k": 0.12,
  "gemini-3-pro-image@4k": 0.2
}
```

如果公开模型名没有配置价格，mihuifang 适配器会尝试使用映射后的上游模型价格。
