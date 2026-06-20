# Mihuifang 异步图片模型调用文档

本文档说明当前系统中三个对外图片模型的调用方式：

- `gpt-image-2`
- `gemini-2.5-flash-image`
- `gemini-3-pro-image`

统一使用异步图片接口：

```http
POST /v1/async/generations
GET /v1/async/generations/{task_id}
```

请求头：

```http
Authorization: Bearer YOUR_API_KEY
Content-Type: application/json
```

## 1. 通用调用规则

### 1.1 创建任务

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
  "quality": "high",
  "n": 1
}
```

### 1.2 查询任务

创建任务后会返回系统公开任务 ID，例如：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "async.generation",
  "type": "image",
  "mode": "text_to_image",
  "status": "pending",
  "progress": 0,
  "detail": {
    "status": "pending"
  }
}
```

用返回的 `task_id` 查询：

```http
GET /v1/async/generations/task_xxx
```

完成后返回：

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

注意：完成后的图片链接为可直接访问的图片地址。

### 1.3 mode 规则

| 场景 | mode | 说明 |
|---|---|---|
| 文生图 | `text_to_image` | 只传 `prompt`，不传图片。 |
| 图生图 | `image_to_image` | 传 `image`、`images` 或 `referenceImages`。 |
| 不传 `mode` | 自动判断 | 有图片输入时自动为 `image_to_image`，否则为 `text_to_image`。 |

### 1.4 size 传参规则

清晰度统一传入 `size`，推荐格式：

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

系统会从 `size` 里识别计费档位：

| size 示例 | 计费档位 |
|---|---|
| `1x1-1k` | `1k` |
| `1x1-2k` | `2k` |
| `1x1-4k` | `4k` |
| `16x9-4k` | `4k` |
| `3840x2160` | `4k` |
| 不传 `size` | 默认 `1k` |

不建议在这三个模型里用 `resolution` 单独传清晰度；对外调用统一使用 `size`，例如 `1x1-4k`。

### 1.5 图片输入字段

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
| `low` | 低质量，价格最低。 |
| `medium` | 中质量，默认推荐。 |
| `high` | 高质量，价格最高。 |

如果不传 `quality`，系统按 `medium` 计费。

### 2.2 支持的 size 写法

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

也可以使用其他比例，例如：

```text
3x2-4k
2x3-4k
5x4-4k
4x5-4k
21x9-4k
```

### 2.3 文生图示例

```bash
curl -X POST "https://你的域名/v1/async/generations" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "mode": "text_to_image",
    "prompt": "一张白色背景的高端香水产品图，柔光摄影，商业广告风格",
    "size": "1x1-4k",
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
    "size": "16x9-2k",
    "quality": "medium",
    "n": 1
  }'
```

### 2.5 价格

| 模型 | size | quality | 价格 |
|---|---|---|---:|
| `gpt-image-2` | 基础价 | - | 0.06 元/次 |
| `gpt-image-2` | `1k` | `low` | 0.06 元/次 |
| `gpt-image-2` | `2k` | `low` | 0.1 元/次 |
| `gpt-image-2` | `4k` | `low` | 0.15 元/次 |
| `gpt-image-2` | `1k` | `medium` | 0.06 元/次 |
| `gpt-image-2` | `2k` | `medium` | 0.1 元/次 |
| `gpt-image-2` | `4k` | `medium` | 0.15 元/次 |
| `gpt-image-2` | `1k` | `high` | 0.1 元/次 |
| `gpt-image-2` | `2k` | `high` | 0.16 元/次 |
| `gpt-image-2` | `4k` | `high` | 0.3 元/次 |

计费示例：

```json
{
  "model": "gpt-image-2",
  "size": "1x1-4k",
  "quality": "high",
  "n": 2
}
```

命中价格：

```text
gpt-image-2@4k@high = 0.3 元/次
```

最终计费：

```text
0.3 * 2 = 0.6 元
```

## 3. 模型二：gemini-2.5-flash-image

### 3.1 适用场景

`gemini-2.5-flash-image` 适合快速图片生成、轻量图生图、社媒图、头像、海报草图等场景。

调用时直接使用 `gemini-2.5-flash-image` 这个模型名。

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

| 模型 | size | 价格 |
|---|---|---:|
| `gemini-2.5-flash-image` | 基础价 | 0.05 元/次 |
| `gemini-2.5-flash-image` | `1k` | 0.05 元/次 |
| `gemini-2.5-flash-image` | `2k` | 0.1 元/次 |
| `gemini-2.5-flash-image` | `4k` | 0.15 元/次 |

计费示例：

```json
{
  "model": "gemini-2.5-flash-image",
  "size": "9x16-4k",
  "n": 3
}
```

命中价格：

```text
gemini-2.5-flash-image@4k = 0.15 元/次
```

最终计费：

```text
0.15 * 3 = 0.45 元
```

## 4. 模型三：gemini-3-pro-image

### 4.1 适用场景

`gemini-3-pro-image` 适合更高质量的图片生成、复杂提示词理解、商业海报、产品图、角色设定图、图像重绘等场景。

调用时直接使用 `gemini-3-pro-image` 这个模型名。

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

| 模型 | size | 价格 |
|---|---|---:|
| `gemini-3-pro-image` | 基础价 | 0.1 元/次 |
| `gemini-3-pro-image` | `1k` | 0.1 元/次 |
| `gemini-3-pro-image` | `2k` | 0.15 元/次 |
| `gemini-3-pro-image` | `4k` | 0.2 元/次 |

计费示例：

```json
{
  "model": "gemini-3-pro-image",
  "size": "16x9-4k",
  "n": 2
}
```

命中价格：

```text
gemini-3-pro-image@4k = 0.2 元/次
```

最终计费：

```text
0.2 * 2 = 0.4 元
```

## 5. 价格汇总

### 5.1 gpt-image-2

| size | quality | 价格 |
|---|---|---:|
| 基础价 | - | 0.06 元/次 |
| `1k` | `low` | 0.06 元/次 |
| `2k` | `low` | 0.1 元/次 |
| `4k` | `low` | 0.15 元/次 |
| `1k` | `medium` | 0.06 元/次 |
| `2k` | `medium` | 0.1 元/次 |
| `4k` | `medium` | 0.15 元/次 |
| `1k` | `high` | 0.1 元/次 |
| `2k` | `high` | 0.16 元/次 |
| `4k` | `high` | 0.3 元/次 |

### 5.2 gemini-2.5-flash-image

| size | 价格 |
|---|---:|
| 基础价 | 0.05 元/次 |
| `1k` | 0.05 元/次 |
| `2k` | 0.1 元/次 |
| `4k` | 0.15 元/次 |

### 5.3 gemini-3-pro-image

| size | 价格 |
|---|---:|
| 基础价 | 0.1 元/次 |
| `1k` | 0.1 元/次 |
| `2k` | 0.15 元/次 |
| `4k` | 0.2 元/次 |

### 5.4 其他图片模型基础价

| 模型 | 价格 |
|---|---:|
| `gemini-3-pro-image-preview` | 0.2 元/次 |
| `gemini-3.1-flash-image` | 0.1333 元/次 |
