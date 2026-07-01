# Mihuifang / Tuyao 异步图片模型集成调用文档

本文档说明当前系统中 Mihuifang 与 Tuyao 两类异步图片渠道的对外调用方式。

当前集成模型：

- `gpt-image-2`
- `gemini-2.5-flash-image`
- `gemini-3-pro-image`
- `tuyao/nanobananapro`

统一使用异步图片接口：

```http
POST /v1/async/generations
GET /v1/tasks/{task_id}
GET /v1/async/generations/{task_id}
```

请求头：

```http
Authorization: Bearer YOUR_API_KEY
Content-Type: application/json
```

## 1. 通用调用规则

### 1.1 渠道路由规则

`/v1/async/generations` 是统一提交入口，系统根据 `model` 自动选择渠道。

| model 写法 | 实际渠道 | 说明 |
|---|---|---|
| `gpt-image-2` | Mihuifang | 系统自动补成 `mihuifang/gpt-image-2`。 |
| `gemini-2.5-flash-image` | Mihuifang | 系统自动补成 `mihuifang/gemini-2.5-flash-image`。 |
| `gemini-3-pro-image` | Mihuifang | 系统自动补成 `mihuifang/gemini-3-pro-image`。 |
| `mihuifang/gpt-image-2` | Mihuifang | 兼容带前缀调用。 |
| `mihuifang/gemini-2.5-flash-image` | Mihuifang | 兼容带前缀调用。 |
| `mihuifang/gemini-3-pro-image` | Mihuifang | 兼容带前缀调用。 |
| `tuyao/nanobananapro` | Tuyao | 必须带 `tuyao/` 前缀才会进入 Tuyao 渠道。 |

不要把系统内部计费 key 当作调用模型名。例如不要传：

```text
mihuifang/gpt-image-2@1k@medium
```

`@1k@medium` 这类后缀只属于系统内部计费匹配，不是对外调用参数。

### 1.2 创建任务

```http
POST /v1/async/generations
```

通用请求结构：

```json
{
  "model": "gpt-image-2",
  "mode": "text_to_image",
  "prompt": "一张产品海报，干净背景，高级商业摄影风格",
  "size": "1x1-4k",
  "quality": "medium",
  "n": 1
}
```

### 1.3 查询任务

创建任务后会返回任务 ID。不同渠道的提交响应字段略有差异：

Mihuifang 常见返回：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "async.generation",
  "type": "image",
  "status": "pending",
  "progress": 0
}
```

Tuyao 常见返回：

```json
{
  "taskOrderId": 2041888888888888888,
  "requestId": "task_xxx",
  "modelCode": "nanobananapro",
  "status": "submitted",
  "billingStatus": "pending",
  "progress": 20
}
```

查询时使用返回里的任务 ID：

- Mihuifang 使用 `task_id` 或 `id`。
- Tuyao 使用 `requestId`。

推荐查询入口：

```http
GET /v1/tasks/task_xxx
```

兼容查询入口：

```http
GET /v1/async/generations/task_xxx
```

推荐使用 `/v1/tasks/{task_id}`。`/v1/async/generations/{task_id}` 保留用于兼容旧客户端。

### 1.4 mode 规则

| 场景 | mode | 说明 |
|---|---|---|
| 文生图 | `text_to_image` | 只传 `prompt`，不传图片。 |
| 图生图 | `image_to_image` | 传 `image`、`images`、`referenceImages` 或 `reference_images`。 |
| 不传 `mode` | 自动判断 | 有图片输入时自动为 `image_to_image`，否则为 `text_to_image`。 |

### 1.5 size 传参规则

不同模型的 `size` 格式不同：

- `gpt-image-2`：建议直接传支持的像素尺寸，例如 `1024x1024`、`2048x1152`、`3840x2160`。
- `gemini-2.5-flash-image` / `gemini-3-pro-image`：继续使用比例-清晰度格式，例如 `1x1-4k`、`16x9-2k`。
- `tuyao/nanobananapro`：兼容 Tuyao 旧格式，例如 `16x9-4K`，也可传上游支持的像素尺寸。

Gemini 系列推荐格式：

```text
比例-清晰度
```

示例：

```text
1x1-1k
1x1-2k
1x1-4k
16x9-1k
16x9-2k
16x9-4k
9x16-1k
9x16-2k
9x16-4k
```

Tuyao 示例：

```text
1x1
16x9-1K
16x9-2K
16x9-4K
9x16-4K
5504x3072
```

### 1.6 图片输入字段

图生图支持以下字段：

```json
{
  "image": "https://example.com/input.png"
}
```

或者：

```json
{
  "images": [
    "https://example.com/input-1.png",
    "https://example.com/input-2.png"
  ]
}
```

或者：

```json
{
  "referenceImages": [
    "https://example.com/reference.png"
  ]
}
```

也兼容 snake_case：

```json
{
  "reference_images": [
    "https://example.com/reference.png"
  ]
}
```

## 2. 模型一：gpt-image-2

### 2.1 适用场景

`gpt-image-2` 适合通用图片生成、商业海报、产品图、图像编辑、参考图重绘等场景。

该模型支持 `quality` 参数：

| quality | 中文说明 |
|---|---|
| `low` | 低质量。 |
| `medium` | 中质量，默认推荐。 |
| `high` | 高质量。 |

当前系统按固定价计费，不再按 `quality` 或清晰度分别收费。

### 2.2 支持的 size 写法

`gpt-image-2` 建议直接传像素尺寸。可参考下表：

| 比例 | 1K | 2K | 4K |
|---|---|---|---|
| `1:1` | `1024x1024` | `2048x2048` | `2880x2880` |
| `16:9` | `1280x720` | `2048x1152` | `3840x2160` |
| `9:16` | `720x1280` | `1152x2048` | `2160x3840` |
| `4:3` | `1152x864` | `2304x1728` | `3264x2448` |
| `3:4` | `864x1152` | `1728x2304` | `2448x3264` |
| `3:2` | `1536x1024` | `2048x1360` | `3504x2336` |
| `2:3` | `1024x1536` | `1360x2048` | `2336x3504` |
| `5:4` | `1120x896` | `2240x1792` | `3200x2560` |
| `4:5` | `896x1120` | `1792x2240` | `2560x3200` |
| `21:9` | `1456x624` | `2912x1248` | `3840x1648` |
| `9:21` | `624x1456` | `1248x2912` | `1648x3840` |

### 2.3 文生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "mode": "text_to_image",
    "prompt": "一张白色背景的高端香水产品图，柔光摄影，商业广告风格",
    "size": "2880x2880",
    "quality": "high",
    "n": 1
  }'
```

### 2.4 图生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "mode": "image_to_image",
    "prompt": "基于参考图生成一张科技感产品海报，保留主体，背景改为深色渐变摄影棚",
    "image": "https://example.com/input.png",
    "size": "2048x1152",
    "quality": "medium",
    "n": 1
  }'
```

### 2.5 价格

| 模型 | 价格 |
|---|---:|
| `gpt-image-2` | 0.08 元/张 |

计费示例：

```json
{
  "model": "gpt-image-2",
  "size": "2880x2880",
  "quality": "high",
  "n": 2
}
```

最终计费：

```text
0.08 * 2 = 0.16 元
```

## 3. 模型二：gemini-2.5-flash-image

### 3.1 适用场景

`gemini-2.5-flash-image` 适合快速图片生成、轻量图生图、社媒图、头像、海报草图等场景。

调用时直接使用 `gemini-2.5-flash-image` 这个模型名，也兼容 `mihuifang/gemini-2.5-flash-image`。

### 3.2 支持的 size 写法

推荐：

```text
1x1-1k
1x1-2k
1x1-4k
16x9-1k
16x9-2k
16x9-4k
9x16-1k
9x16-2k
9x16-4k
4x3-1k
4x3-2k
4x3-4k
3x4-1k
3x4-2k
3x4-4k
```

也可以使用超长比例：

```text
1x8-1k
1x8-2k
1x8-4k
1x4-1k
1x4-2k
1x4-4k
4x1-1k
4x1-2k
4x1-4k
8x1-1k
8x1-2k
8x1-4k
```

### 3.3 文生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "mode": "text_to_image",
    "prompt": "一张赛博朋克风格的城市夜景插画，霓虹灯，高细节",
    "size": "16x9-2k",
    "n": 1
  }'
```

### 3.4 图生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "mode": "image_to_image",
    "prompt": "把参考图转成日系动画电影海报风格，保留人物姿态和构图",
    "images": [
      "https://example.com/input.png"
    ],
    "size": "9x16-4k",
    "n": 1
  }'
```

### 3.5 价格

| 模型 | 价格 |
|---|---:|
| `gemini-2.5-flash-image` | 0.08 元/张 |

计费示例：

```json
{
  "model": "gemini-2.5-flash-image",
  "size": "9x16-4k",
  "n": 3
}
```

最终计费：

```text
0.08 * 3 = 0.24 元
```

## 4. 模型三：gemini-3-pro-image

### 4.1 适用场景

`gemini-3-pro-image` 适合更高质量的图片生成、复杂提示词理解、商业海报、产品图、角色设定图、图像重绘等场景。

调用时直接使用 `gemini-3-pro-image` 这个模型名，也兼容 `mihuifang/gemini-3-pro-image`。

### 4.2 支持的 size 写法

推荐：

```text
1x1-1k
1x1-2k
1x1-4k
16x9-1k
16x9-2k
16x9-4k
9x16-1k
9x16-2k
9x16-4k
4x3-1k
4x3-2k
4x3-4k
3x4-1k
3x4-2k
3x4-4k
```

### 4.3 文生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3-pro-image",
    "mode": "text_to_image",
    "prompt": "一张高端电动汽车发布会主视觉海报，黑色车身，未来科技感，电影级灯光",
    "size": "16x9-4k",
    "n": 1
  }'
```

### 4.4 图生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3-pro-image",
    "mode": "image_to_image",
    "prompt": "根据参考图生成一张奢侈品广告大片，保留产品外形，背景换成高级摄影棚",
    "referenceImages": [
      "https://example.com/product.png"
    ],
    "size": "1x1-4k",
    "n": 1
  }'
```

### 4.5 价格

| 模型 | 价格 |
|---|---:|
| `gemini-3-pro-image` | 0.25 元/张 |

计费示例：

```json
{
  "model": "gemini-3-pro-image",
  "size": "16x9-4k",
  "n": 2
}
```

最终计费：

```text
0.25 * 2 = 0.5 元
```

## 5. 模型四：tuyao/nanobananapro

### 5.1 适用场景

`tuyao/nanobananapro` 使用独立 Tuyao 渠道，适合 NanoBanana Pro 图片生成、图生图和高分辨率图片任务。

该模型必须带 `tuyao/` 前缀调用：

```text
tuyao/nanobananapro
```

系统转发到 Tuyao 上游时会自动去掉前缀，上游实际收到：

```text
nanobananapro
```

### 5.2 支持的 size 写法

推荐：

```text
1x1
16x9-1K
16x9-2K
16x9-4K
9x16-1K
9x16-2K
9x16-4K
```

也可以按上游支持直接传像素尺寸：

```text
5504x3072
3072x5504
2048x2048
```

### 5.3 文生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
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

### 5.4 图生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
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

### 5.5 提交响应

Tuyao 返回的任务 ID 字段是 `requestId`：

```json
{
  "taskOrderId": 2041888888888888888,
  "requestId": "task_xxx",
  "modelCode": "nanobananapro",
  "status": "submitted",
  "billingStatus": "pending",
  "progress": 20
}
```

查询时使用 `requestId`：

```http
GET /v1/tasks/task_xxx
```

### 5.6 价格

Tuyao 模型按本系统公开模型名配置价格：

| 模型 | 价格 |
|---|---:|
| `tuyao/nanobananapro` | 以后台配置为准，示例 0.08 元/张 |

示例价格配置：

```json
{
  "tuyao/nanobananapro": 0.08
}
```

## 6. 查询响应格式

### 6.1 Mihuifang 完成响应

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "async.generation",
  "type": "image",
  "mode": "text_to_image",
  "status": "completed",
  "progress": 100,
  "result": {
    "image_url": "https://your-oss-domain/path/image.png",
    "url": "https://your-oss-domain/path/image.png"
  },
  "url": "https://your-oss-domain/path/image.png",
  "detail": {
    "status": "completed"
  }
}
```

### 6.2 Tuyao 查询响应

本系统查询接口会返回统一包装，`data` 内是上游任务结果：

```json
{
  "code": "success",
  "data": {
    "requestId": "task_xxx",
    "modelCode": "nanobananapro",
    "status": "succeeded",
    "progress": 100,
    "resultCount": 1,
    "result": {
      "image_url": "https://your-oss-domain/path/image.png",
      "url": "https://your-oss-domain/path/image.png",
      "items": [
        {
          "url": "https://your-oss-domain/path/image.png",
          "type": "image"
        }
      ]
    },
    "url": "https://your-oss-domain/path/image.png"
  }
}
```

### 6.3 状态说明

| 渠道状态 | 本系统含义 |
|---|---|
| `pending` / `submitted` / `queued` | 已提交或排队中。 |
| `processing` / `running` / `in_progress` | 处理中。 |
| `completed` / `complete` / `success` / `succeeded` | 成功。 |
| `failed` / `failure` / `error` | 失败。 |

## 7. 价格汇总

### 7.1 固定价格

| 模型 | 单价 |
|---|---:|
| `gpt-image-2` | 0.08 元/张 |
| `gemini-2.5-flash-image` | 0.08 元/张 |
| `gemini-3-pro-image` | 0.25 元/张 |
| `tuyao/nanobananapro` | 以后台配置为准，示例 0.08 元/张 |

计费规则：

```text
单价 * 成功图片数
```

任务提交时会先按 `n` 预扣，任务完成后按成功数量结算并退还差额。

### 7.2 后台价格 JSON 示例

Mihuifang 固定价配置示例：

```json
{
  "mihuifang/gpt-image-2": 0.08,
  "mihuifang/gemini-2.5-flash-image": 0.08,
  "mihuifang/gemini-3-pro-image": 0.25
}
```

包含 Tuyao 的配置示例：

```json
{
  "mihuifang/gpt-image-2": 0.08,
  "mihuifang/gemini-2.5-flash-image": 0.08,
  "mihuifang/gemini-3-pro-image": 0.25,
  "tuyao/nanobananapro": 0.08
}
```

如果后台仍保留 `mihuifang/gpt-image-2@1k@medium` 这类旧阶梯价格 key，可以继续兼容历史任务；但对外调用方不需要、也不应该传这些 key。

## 8. 常见问题

### 8.1 为什么 Tuyao 不能复用 Mihuifang provider

Mihuifang 提交响应用：

```json
{
  "task_id": "task_xxx",
  "id": "task_xxx"
}
```

Tuyao 提交响应用：

```json
{
  "requestId": "task_xxx"
}
```

字段不一致时，旧 Mihuifang provider 取不到任务 ID，会返回：

```json
{
  "code": "invalid_response",
  "message": "task id missing in response",
  "data": null
}
```

所以系统中已经把 Tuyao 独立为 `tuyao` 渠道。

### 8.2 什么时候用 `/v1/tasks/{task_id}`

新客户端推荐统一使用：

```http
GET /v1/tasks/{task_id}
```

旧客户端仍可继续使用：

```http
GET /v1/async/generations/{task_id}
```

两个查询入口都会根据本地任务记录找到原始渠道，再调用对应上游查询接口。
