# OpenAI 图片与视频兼容接口 API 文档

> 来源：`https://mingyu.it.com/openai-image-compat-examples.html#async-generations`，整理时间：2026-06-20。
> 示例服务地址统一使用 `https://mingyu.it.com`，请求头统一使用 `Authorization: Bearer your_api_key`。

## 1. 通用说明

### 1.1 认证与请求格式

- JSON 请求：`Content-Type: application/json`
- 文件上传：`multipart/form-data`
- 图片结果：官方图片端点返回 `created` 与 `data`，图片链接在 `data[0].url`，Base64 在 `data[0].b64_json`。
- Chat 兼容图片/视频结果：返回 `chat.completion`，结果通常在 `choices[0].message.content` 中以 Markdown 图片或 HTML video 返回。
- 官方视频端点：创建和查询返回 `id`、`object`、`model`、`status`、`progress`、`created_at`；视频内容通过 `GET /v1/videos/{video_id}/content` 获取。

### 1.2 尺寸、比例与清晰度写法

| 参数 | 写法 | 说明 |
|---|---|---|
| `size` | `1x1`、`16x9`、`9x16` | 只写比例时默认按 `1K` 处理。 |
| `size` | `16x9-1k`、`16x9-2k`、`16x9-4k` | 推荐的“比例 + 清晰度档位”写法。 |
| `size` | `2k-16x9`、`4k-1x1` | 兼容“清晰度档位 + 比例”写法。 |
| `size` | `1024x1024`、`2048x2048`、`3840x2160` | 像素写法，系统按最大边映射到 `1K` / `2K` / `4K`。 |
| `size` | `auto`、`auto-2k`、`auto-4k` | 文生图默认 `1K 1:1`；图生图会按输入图片宽高匹配最近比例，并保留档位。 |
| `aspect_ratio` | `1:1`、`16:9`、`9:16`、`4:3`、`3:4` | 异步、视频兼容和部分上游格式使用。 |
| `resolution` | `1K`、`2K`、`4K` | Gemini 图片兼容和异步图片可用。 |
| `resolution` | `720p`、`1080p` | Veo、Seedance、Abra 视频清晰度。 |
| `quality` | `low`、`medium`、`high` | 仅 `gpt-image-2` 图片模型使用，不是通用清晰度字段。 |

### 1.3 图片比例速查

| 模型 | 支持比例 |
|---|---|
| `gpt-image-2` | `auto`、`1x1`、`16x9`、`9x16`、`4x3`、`3x4`、`3x2`、`2x3`、`5x4`、`4x5`、`21x9`，可追加 `-1k` / `-2k` / `-4k`。 |
| `nano-banana` | `auto`、`1x1`、`16x9`、`9x16`、`4x3`、`3x4`，可追加 `-1k` / `-2k` / `-4k`。 |
| `nano-banana-pro` | `auto`、`1x1`、`16x9`、`9x16`、`4x3`、`3x4`，可追加 `-1k` / `-2k` / `-4k`。 |
| `nano-banana2` | `auto`、`1x1`、`16x9`、`9x16`、`4x3`、`3x4`、`1x8`、`1x4`、`4x1`、`8x1`，可追加 `-1k` / `-2k` / `-4k`。 |

## 2. 模型与端点总览

| 场景 | 模型 | 推荐端点 | 关键参数 |
|---|---|---|---|
| GPT 图片生成 | `gpt-image-2` | `POST /v1/images/generations` | `prompt`、`size`、`quality`、`response_format` |
| GPT 图片编辑 | `gpt-image-2` | `POST /v1/images/edits` | `prompt`、`image`、`images`、`referenceImages`、`size`、`quality` |
| GPT 图片 Chat 兼容 | `gpt-image-2` | `POST /v1/chat/completions` | `prompt` 或 `messages`、`size`、`quality` |
| Nano Banana 图片 | `nano-banana`、`nano-banana2`、`nano-banana-pro` | `POST /v1/chat/completions` | `prompt` 或 `messages`、`size` |
| Gemini 图片兼容 | `nano-banana`、`nano-banana2`、`nano-banana-pro` | `POST /v1beta/models/{model}:generateContent` | `contents`、`generationConfig.imageConfig.aspectRatio`、`imageSize` |
| 统一异步 | 图片与视频模型 | `POST /v1/async/generations`、`GET /v1/async/generations/{task_id}` | `model`、`mode`、`prompt`、`images`、`size`、`aspect_ratio`、`resolution`、`duration` |
| Sora 视频 | `sora2`、`sora2-pro` | `POST /v1/videos` | `prompt`、`size`、`duration` / `seconds`、`image` |
| Veo 视频 | `veo31`、`veo31-ref`、`veo31-fast` | `POST /v1/videos` | `prompt`、`size`、`duration`、`image` |
| Grok 图生视频 | `grok-imagine-video-1.5-preview`、`grok-imagine-1.0-video` | `POST /v1/video/create`、`GET /v1/video/query?id=...` | `prompt`、`image` / `images`、`seconds`、`size`、`aspect_ratio` |
| Abra 视频 | `abra_omni`、`abra_omni_per_second` 等 | `POST /v1/video/create`、`GET /v1/video/query?id=...` | `mode`、`prompt`、`seconds`、`aspect_ratio`、`resolution`、`video_url`、`images` |
| Seedance 2.0 | `doubao-seedance-2-0-260128`、`doubao-seedance-2-0-fast-260128` | `POST /v1/video/generations`、`GET /v1/video/generations/{task_id}` | `prompt`、`image` / `images`、`metadata.content`、`seconds`、`resolution`、`aspect_ratio` |

## 3. 图片接口

### 3.1 GPT 图片生成

`POST /v1/images/generations`

```json
{
  "model": "gpt-image-2",
  "prompt": "A simple blue compass icon on white background",
  "size": "1x1",
  "quality": "medium"
}
```

| 参数 | 必填 | 可选值 / 格式 | 说明 |
|---|---:|---|---|
| `model` | 是 | `gpt-image-2` | 固定模型。 |
| `prompt` | 是 | string | 文本提示词。 |
| `size` | 否 | `auto`、比例、比例档位、像素 | 例如 `1x1`、`16x9-2k`、`3840x2160`。 |
| `quality` | 否 | `low`、`medium`、`high` | 仅 GPT 图片模型生效。 |
| `response_format` | 否 | `url`、`b64_json` | 默认返回 URL；渠道支持时可返回 `b64_json`。 |

### 3.2 GPT 图片编辑

`POST /v1/images/edits`

支持两种输入：

- `multipart/form-data`：文件字段名固定为 `image`。
- JSON：`image` / `images` / `referenceImages` 传 data URL、原始 Base64 或图片 URL。

```json
{
  "model": "gpt-image-2",
  "prompt": "Turn this image into a clean product icon",
  "size": "auto",
  "quality": "medium",
  "image": "data:image/png;base64,...",
  "referenceImages": [
    {"url": "data:image/png;base64,..."}
  ]
}
```

| 参数 | 必填 | 可选值 / 格式 | 说明 |
|---|---:|---|---|
| `model` | 是 | `gpt-image-2` | 固定模型。 |
| `prompt` | 是 | string | 编辑或重绘指令。 |
| `image` | 是 | 文件 / data URL / Base64 / URL | multipart 文件字段名为 `image`；JSON 可传字符串。 |
| `images` | 否 | string[] | 多图输入。 |
| `referenceImages` | 否 | string[] / object[] | legacy 兼容字段，会映射到现有图片输入。 |
| `size` | 否 | `auto`、比例、比例档位、像素 | `auto` 会根据输入图匹配最近比例。 |
| `quality` | 否 | `low`、`medium`、`high` | 仅 GPT 图片模型生效。 |

### 3.3 GPT 图片 Chat 兼容入口

`POST /v1/chat/completions`

文生图可以直接传顶层 `prompt`：

```json
{
  "model": "gpt-image-2",
  "prompt": "A simple blue compass icon on white background",
  "size": "9x16",
  "quality": "medium"
}
```

图生图推荐使用多模态 `messages[].content[].image_url`：

```json
{
  "model": "gpt-image-2",
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "把参考图改成科技产品海报，保留主体轮廓"},
        {"type": "image_url", "image_url": {"url": "data:image/png;base64,..."}}
      ]
    }
  ],
  "size": "16x9",
  "quality": "medium"
}
```

| 参数 | 必填 | 可选值 / 格式 | 说明 |
|---|---:|---|---|
| `model` | 是 | `gpt-image-2` | 固定模型。 |
| `prompt` | 条件 | string | 文生图可用。 |
| `messages[].content[].text` | 条件 | string | 图生图编辑指令。 |
| `messages[].content[].image_url.url` | 条件 | data URL / URL | 图生图参考图，推荐 data URL。 |
| `size` | 否 | `auto`、比例、比例档位、像素 | 例如 `21x9-4k`。 |
| `quality` | 否 | `low`、`medium`、`high` | 仅 GPT 图片模型生效。 |

### 3.4 Nano Banana Chat 兼容入口

`POST /v1/chat/completions`

> Nano Banana 系列只使用 `/v1/chat/completions`，不要调用 `/v1/images/generations` 或 `/v1/images/edits`。

```json
{
  "model": "nano-banana2",
  "prompt": "a red square",
  "size": "1x1"
}
```

图生图同样使用 `messages[].content[].image_url`：

```json
{
  "model": "nano-banana2",
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "把参考图改成电影海报风格，保留主体构图"},
        {"type": "image_url", "image_url": {"url": "data:image/png;base64,..."}}
      ]
    }
  ],
  "size": "1x1"
}
```

| 模型 | `size` 支持 | 说明 |
|---|---|---|
| `nano-banana` | `auto`、`1x1`、`16x9`、`9x16`、`4x3`、`3x4`，可追加 `-1k` / `-2k` / `-4k` | 支持像素写法。 |
| `nano-banana-pro` | `auto`、`1x1`、`16x9`、`9x16`、`4x3`、`3x4`，可追加 `-1k` / `-2k` / `-4k` | 支持像素写法。 |
| `nano-banana2` | `auto`、`1x1`、`16x9`、`9x16`、`4x3`、`3x4`、`1x8`、`1x4`、`4x1`、`8x1`，可追加 `-1k` / `-2k` / `-4k` | 额外支持超长比例。 |

### 3.5 Gemini 官方图片生成兼容

`POST /v1beta/models/{model}:generateContent`

```json
{
  "contents": [
    {
      "role": "user",
      "parts": [
        {"text": "A cinematic banana mascot holding a paint brush"}
      ]
    }
  ],
  "generationConfig": {
    "responseModalities": ["TEXT", "IMAGE"],
    "imageConfig": {
      "aspectRatio": "16:9",
      "imageSize": "4K"
    }
  }
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `{model}` | `nano-banana`、`nano-banana2`、`nano-banana-pro` | 路径中的模型名。 |
| `contents[].parts[].text` | string | 文本提示词。 |
| `contents[].parts[].inlineData` | `{mimeType,data}` | 参考图 Base64；兼容 `inline_data.mime_type`。 |
| `contents[].parts[].fileData` | `{mimeType,fileUri}` | 参考图 URL；兼容 `file_data.file_uri`。 |
| `generationConfig.imageConfig.aspectRatio` | `1:1`、`16:9`、`9:16`、`4:3`、`3:4` | `nano-banana2` 额外支持 `1:8`、`1:4`、`4:1`、`8:1`。 |
| `generationConfig.imageConfig.imageSize` | `1K`、`2K`、`4K` | 与比例组合为内部 `size`，如 `16:9 + 4K` => `16x9-4k`。 |
| `generation_config.image_config` | snake_case | 兼容 `aspect_ratio` / `image_size`。 |

## 4. 统一异步入口

### 4.1 创建任务

`POST /v1/async/generations`

```json
{
  "model": "gpt-image-2",
  "mode": "text_to_image",
  "prompt": "A clean studio photo of a green ceramic bowl on a white kitchen counter",
  "size": "1x1",
  "quality": "low",
  "n": 1
}
```

### 4.2 查询任务

`GET /v1/async/generations/{task_id}`

图片任务完成时，结果地址优先读取：

- `result.image_url`
- `result.image_urls[]`
- `url`

视频任务完成时，结果地址优先读取：

- `result.video_url`
- `url`

### 4.3 异步参数

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | 图片：`gpt-image-2`、`nano-banana`、`nano-banana2`、`nano-banana-pro`；视频：`sora2`、`sora2-pro`、`veo31`、`veo31-ref`、`veo31-fast`、`grok-imagine-video-1.5-preview`、`grok-imagine-1.0-video` | 统一异步支持的模型。 |
| `mode` | `text_to_image`、`image_to_image` | `gpt-image-2` 可选；有图片输入时默认图生图。兼容 `edit`、`image_edit`、`img2img`。 |
| `prompt` | string | 提示词；也可从 `messages[].content[]` 提取。 |
| `images` | string[] / object[] | 图生图输入，支持 URL、data URL、原始 Base64、`{"url":"..."}`、`{"image_url":"..."}`。 |
| `image` | string | 单图输入。 |
| `referenceImages` / `reference_images` | array | 兼容图生图输入。 |
| `size` | 比例、比例档位、像素、视频 size | 可直接传 `16x9-2k`、`16x9-720p`。 |
| `aspect_ratio` | `1:1`、`16:9`、`9:16`、`4:3`、`3:4` | 图片和视频都可用。 |
| `resolution` | 图片：`1K` / `2K` / `4K`；Veo：`720p` / `1080p` | 也可直接用 `size` 表达。 |
| `quality` | `low`、`medium`、`high` | `gpt-image-2` 图片任务使用。 |
| `duration` / `seconds` | Sora：`4` / `8` / `12`；Veo：`4` / `6` / `8` | 视频时长。 |
| `n` | number | 生成数量。 |

### 4.4 异步图生图示例

```json
{
  "model": "gpt-image-2",
  "mode": "image_to_image",
  "prompt": "Turn this product photo into a clean studio ad image, no text",
  "images": ["data:image/png;base64,..."],
  "size": "1x1",
  "quality": "low",
  "n": 1
}
```

Nano Banana 异步示例：

```json
{
  "model": "nano-banana2",
  "mode": "text_to_image",
  "prompt": "A cinematic product photo of a banana-shaped ceramic lamp on a walnut table",
  "aspect_ratio": "4:3",
  "resolution": "2K",
  "n": 1
}
```

## 5. 视频接口

### 5.1 官方视频端点：Sora

`POST /v1/videos`

```json
{
  "model": "sora2",
  "prompt": "A slow aerial shot over a quiet neon city at night",
  "size": "16x9",
  "seconds": "4"
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | `sora2`、`sora2-pro` | Sora 模型。 |
| `prompt` | string | 文本提示词。 |
| `size` | `16x9`、`9x16` | 视频比例。 |
| `duration` / `seconds` | `4`、`8`、`12` | 视频时长。 |
| `image` | data URL / Base64 / URL | 可选图生视频参考图。 |
| `images` | string[] | 多张参考图；不同上游支持程度不同。 |

查询：

- `GET /v1/videos/{video_id}` 查询任务。
- `GET /v1/videos/{video_id}/content` 获取视频文件。

### 5.2 官方视频端点：Veo

`POST /v1/videos`

```json
{
  "model": "veo31-fast",
  "prompt": "A handheld documentary shot of waves hitting black rocks",
  "size": "16x9-720p",
  "duration": 4
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | `veo31`、`veo31-ref`、`veo31-fast` | Veo 模型。 |
| `prompt` | string | 文本提示词。 |
| `size` | `16x9-720p`、`16x9-1080p`、`9x16-720p`、`9x16-1080p` | 同时表达比例和清晰度。 |
| `duration` / `seconds` | `4`、`6`、`8` | `veo31-ref` 当前建议使用 `8`。 |
| `image` | data URL / Base64 / URL | 可选图生视频参考图，`veo31-ref` 常用。 |
| `images` | string[] | 多张参考图。 |

### 5.3 Chat Completions 视频兼容入口

`POST /v1/chat/completions`

```json
{
  "model": "sora2",
  "messages": [
    {"role": "user", "content": "A slow aerial shot over a quiet neon city at night"}
  ],
  "size": "16x9",
  "duration": 4
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | `sora2`、`sora2-pro`、`veo31`、`veo31-ref`、`veo31-fast` | Chat 兼容视频模型。 |
| `prompt` / `messages` | string / messages | 提示词。 |
| `size` | Sora：`16x9` / `9x16`；Veo：`16x9-720p` 等 | 视频比例/清晰度。 |
| `duration` | Sora：`4` / `8` / `12`；Veo：`4` / `6` / `8` | 视频时长。 |
| `image` / `images` | data URL / Base64 / URL | 图生视频参考图。 |

### 5.4 lnapi 视频兼容入口

创建：`POST /v1/video/create`

查询：`GET /v1/video/query?id={task_id}`

```json
{
  "model": "veo31-fast",
  "prompt": "A handheld documentary shot of waves hitting black rocks",
  "duration": 4,
  "aspect_ratio": "16:9",
  "size": "16x9-720p",
  "enhance_prompt": true
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | `sora2`、`sora2-pro`、`veo31`、`veo31-ref`、`veo31-fast`、`grok-imagine-video-1.5-preview`、`grok-imagine-1.0-video`，或映射后的上游模型 | 兼容视频模型。 |
| `duration` / `seconds` | number / string | 视频时长；Grok 中只控制时长，不作为计费倍率。 |
| `aspect_ratio` | `16:9`、`9:16`、`1:1` | Veo 优先使用；Grok 会映射为像素 `size`。 |
| `size` | Veo size 或 Grok 像素尺寸 | Grok 推荐 `1280x720`、`720x1280`、`1024x1024`、`1792x1024`、`1024x1792`。 |
| `enhance_prompt` | boolean / string boolean | 显式 `false` 会保留。 |
| `enable_upsample` | boolean / string boolean | 透传到任务 metadata。 |
| `image` / `images` | data URL / URL | 图生视频参考图。 |

### 5.5 Grok 图生视频

推荐使用 lnapi 异步格式。

```json
{
  "model": "grok-imagine-video-1.5-preview",
  "prompt": "保留主体和构图，让画面自然动起来，轻微电影感运镜，环境有细微动态。不要文字，不要水印。",
  "images": ["data:image/png;base64,..."],
  "seconds": "15",
  "size": "1280x720",
  "aspect_ratio": "16:9"
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | `grok-imagine-video-1.5-preview`、`grok-imagine-1.0-video` | 1.5 只支持单参考图；1.0 支持多参考图。 |
| `prompt` | string | 视频动作提示词。 |
| `image` / `images` | data URL / URL | 参考图；1.5 多图只取第一张。 |
| `reference_images` | array | 兼容字段，新接入优先用 `images`。 |
| `seconds` | 1.5 最多 `15`；1.0 最多 `10` | 控制生成时长，不作为计费倍率。 |
| `size` | `1280x720`、`720x1280`、`1024x1024`、`1792x1024`、`1024x1792` | 推荐显式像素尺寸。 |
| `aspect_ratio` | `16:9`、`9:16`、`1:1` | 未传 `size` 时会自动映射。 |
| `resolution` | string | 兼容字段，通常不用传。 |
| `preset` | `normal` | 兼容字段。 |
| `video_config.video_length` | number / string | 兼容字段；同时传 `seconds` 时以实际任务解析秒数为准。 |

比例到像素映射：

| 比例 | Grok `size` |
|---|---|
| `16:9` / `16x9` | `1280x720` |
| `9:16` / `9x16` | `720x1280` |
| `1:1` / `1x1` | `1024x1024` |

### 5.6 Abra 视频

创建：`POST /v1/video/create`

查询：`GET /v1/video/query?id={task_id}`

推荐优先使用 `abra_omni`；客户端需要按秒计费入口时使用 `abra_omni_per_second`。

```json
{
  "model": "abra_omni",
  "mode": "r2v",
  "prompt": "Animate this character walking through a neon street, cinematic lighting",
  "seconds": 6,
  "aspect_ratio": "9:16",
  "images": ["data:image/png;base64,..."]
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | `abra_omni`、`abra_omni_per_second`、`abra_t2v_4s/6s/8s/10s`、`abra_r2v_4s/6s/8s/10s`、`abra_edit` 及 `_1080p` 变体 | 推荐 `abra_omni`。 |
| `mode` | `t2v`、`r2v`、`edit` | `abra_omni` 使用；分别表示文生视频、图生视频、视频编辑。 |
| `seconds` / `duration` | `4`、`6`、`8`、`10` | T2V/R2V 支持；非精确值向上归档。Edit 固定按 `10` 秒处理。 |
| `aspect_ratio` | `16:9` / `landscape`、`9:16` / `portrait` | 也兼容 `generationConfig.aspectRatio`。 |
| `resolution` | `1080p` | 使用 1080p 变体。 |
| `enable_upsample` | `true` / `false` | `true` 时使用 1080p 变体。 |
| `video_url` / `video` | data URL / 可下载 URL | Abra 编辑源视频，推荐 `data:video/mp4;base64,...`。 |
| `video_filename` | string | 可选源视频文件名。 |
| `images` | data URL[] | R2V 最多 3 张；Edit 为 1 个源视频加最多 3 张参考图。 |

1080p 写法：

```json
{
  "model": "abra_omni",
  "mode": "t2v",
  "prompt": "A cinematic aerial shot of a futuristic city at night",
  "seconds": 8,
  "resolution": "1080p"
}
```

### 5.7 Seedance 2.0 视频

创建：`POST /v1/video/generations`

查询：`GET /v1/video/generations/{task_id}`

```json
{
  "model": "doubao-seedance-2-0-fast-260128",
  "prompt": "A cat running on grass, stable camera",
  "seconds": 5,
  "resolution": "720p",
  "aspect_ratio": "16:9"
}
```

| 参数 | 可选值 / 格式 | 说明 |
|---|---|---|
| `model` | `doubao-seedance-2-0-260128`、`doubao-seedance-2-0-fast-260128` | Seedance 2.0 模型。 |
| `prompt` | string | 文本提示词。 |
| `seconds` / `duration` | number | 默认 `5` 秒，计费按实际传入秒数计算。 |
| `resolution` | `720p`、`1080p` | 不传默认按 `720p` 计费。 |
| `aspect_ratio` / `metadata.ratio` | `16:9`、`9:16` 等 | 视频比例。 |
| `image` / `images` | URL / data URL / Base64 / `asset://asset-xxxxx` | 普通图生视频参考图，会按 `reference_image` 转发。 |
| `metadata.content` | content[] | 上游 content 数组，支持 `text`、`image_url`、`video_url`、`audio_url`。 |
| `metadata.content[].role` | `reference_image`、`first_frame`、`last_frame` | 多图上游 content 写法必须明确 role。 |
| `metadata.generate_audio` | boolean | 使用 `audio_url` 音频参考口播时建议设为 `true`。 |
| `metadata.watermark` | boolean | 是否添加水印，默认 `false`。 |

首尾帧示例：

```json
{
  "model": "doubao-seedance-2-0-fast-260128",
  "prompt": "从首帧画面自然运动到尾帧画面，动作连贯，镜头稳定",
  "seconds": 5,
  "resolution": "720p",
  "aspect_ratio": "16:9",
  "metadata": {
    "content": [
      {"type": "image_url", "role": "first_frame", "image_url": {"url": "https://example.com/start.jpg"}},
      {"type": "image_url", "role": "last_frame", "image_url": {"url": "https://example.com/end.jpg"}}
    ]
  }
}
```

音频参考说明：音频参考必须搭配图片或视频使用，推荐“文本 + 图片 + 音频”或“文本 + 视频 + 音频”，不支持纯音频。

## 6. 参考图与素材字段

### 6.1 图片 / 视频参考图

| 字段 | 适用接口 | 说明 |
|---|---|---|
| `image` | 图片编辑、视频图生视频、异步 | 单张参考图；推荐 `data:image/png;base64,...`、`data:image/jpeg;base64,...`、`data:image/webp;base64,...`。 |
| `images` | 图片编辑、视频图生视频、异步 | 多张参考图数组；不同上游对多图支持不同，单图最稳。 |
| `input_reference` | 视频兼容 | 兼容字段，可传 Base64。 |
| `referenceImages` / `reference_images` | 图片编辑、视频兼容、异步 | legacy 兼容字段，会尽量转为 `images` 或 `referenceBlobs`。 |
| `referenceBlobs` | Sora 原生 | 已有上游 storage blob ID 时使用，每项包含 `id`、`usage`，可选 `presignedUrl`。 |
| `messages[].content[].image_url.url` | GPT 图片和 Nano Banana 图生图 | 推荐 data URL。 |

### 6.2 Seedance 素材库

素材接口：`/volc/ark?Action=...`

| Action | 用途 |
|---|---|
| `CreateAssetGroup` | 创建虚拟人像分组。 |
| `CreateVisualValidateSession` | 创建真人人像 H5 认证链接。 |
| `GetVisualValidateResult` | 通过 `BytedToken` 获取 `GroupId`。 |
| `CreateAsset` | 上传图片、视频或音频素材。 |
| `GetAsset` | 查询素材状态。 |

| 字段 / 规则 | 说明 |
|---|---|
| `AssetType` | `Image`、`Video`、`Audio`。 |
| 图片规格 | 支持 `jpeg`、`png`、`webp`、`bmp`、`tiff`、`gif`、`heic/heif`；宽高比 0.4 到 2.5；宽高 300 到 6000 px；大小小于 30 MB。 |
| `ProjectName` | 素材分组、上传、查询和生成任务的 API Key 所属项目必须一致；不传默认 `default`。 |
| 素材引用 | 使用 `asset://asset-xxxxx` 放入 `metadata.content[].image_url.url` / `audio_url.url`。 |
| Prompt 引用 | 用“图片1”“音频1”等素材类型 + 序号描述参考素材，不要在 prompt 中直接写 Asset ID 或 URL。 |

## 7. 价格与计费参数

| 模型 / 系列 | 计费规则 |
|---|---|
| `gpt-image-2`，`quality=low` / `medium` | 按原尺寸档计价：`1K=0.03`、`2K=0.05`、`4K=0.075`。 |
| `gpt-image-2`，`quality=high` | 按高质量尺寸档计价：`1K=0.05`、`2K=0.08`、`4K=0.15`。 |
| Nano Banana 系列 | 价格由 `model` 与 `size` 解析到对应尺寸和比例。 |
| Seedance 2.0 Fast | `doubao-seedance-2-0-fast-260128`：`720p=0.63 元/秒`、`1080p=0.73 元/秒`。 |
| Seedance 2.0 标准 | `doubao-seedance-2-0-260128`：`720p=0.82 元/秒`、`1080p=0.92 元/秒`。 |
| Seedance 素材库 | `kyc-asset` 按 `0.01 元/次` 计费。 |
| Sora 系列 | 价格由 `model`、`duration` 和 `size` 解析。 |
| Veo 系列 | 价格由 `model`、`duration`、`size` 中的比例与分辨率解析。 |
| Grok 视频 | 按模型配置的单次价格计费，`seconds` 不再额外乘到费用里。 |
| Abra `abra_omni_per_second` | T2V/R2V 按映射后的 `4` / `6` / `8` / `10` 秒计费；Edit 固定按 `10` 秒计费。 |

## 8. 快速选择建议

- GPT 图片：需要官方图片返回结构时用 `/v1/images/generations` / `/v1/images/edits`；需要统一 Chat 结构时用 `/v1/chat/completions`。
- Nano Banana：只用 `/v1/chat/completions` 或 Gemini 兼容 `/v1beta/models/{model}:generateContent`。
- 图片清晰度：优先用 `size` 的 `-1k` / `-2k` / `-4k` 表达；`quality` 只用于 `gpt-image-2`。
- 视频清晰度：Veo 用 `size=16x9-720p` 这类组合；Seedance / Abra 可用 `resolution=720p|1080p`。
- 视频异步：Sora / Veo 官方风格用 `/v1/videos`；lnapi 兼容或 Grok / Abra 用 `/v1/video/create`；统一任务系统用 `/v1/async/generations`。
