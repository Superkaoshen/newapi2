# AIAPIPro 图片模型官方参考文档

本文档记录 AIAPIPro 图片模型的官方调用规则。后续修改 `relay/channel/task/mihuifang` 适配器前，必须先核对本文档中的模型字段、尺寸表、参考图限制和计费档位。

客户端调用本系统时使用本系统域名；本文中的 `https://aiapipro.vip` 是上游官方示例地址，不应暴露给普通用户作为本系统调用地址。

## 通用接入规则

### 鉴权

```http
Authorization: Bearer sk_xxx
```

### 接入流程

1. `GET /v1/models` 获取可用模型列表。
2. `POST /v1/images/generations` 或 `POST /v1/images/edits` 提交异步任务，并保存返回的 `requestId`。
3. `GET /v1/tasks/{requestId}` 轮询任务状态，直到任务完成。

### OpenAI 兼容接口

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/v1/models` | 获取当前支持模型。 |
| `POST` | `/v1/images/generations` | 图片生成。 |
| `POST` | `/v1/images/edits` | 图片编辑，支持 JSON 和 multipart/form-data。 |
| `GET` | `/v1/tasks/{requestId}` | 查询异步任务。 |

### 通用字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `model` | string | 是 | 模型代码，例如 `nanobananapro`。 |
| `prompt` | string | 是 | 生成或编辑提示词。 |
| `n` | number | 否 | 输出数量，建议传 `1`。 |
| `size` | string | 否 | OpenAI 兼容像素尺寸，例如 `1584x672`。不要传 `9x16-4k`。 |
| `quality` | string | 否 | 根据模型和输出档位传值。Nano 系列为 `standard`/`hd`，`gpt-image-2` 为 `low`/`medium`/`high`。 |
| `response_format` | string | 否 | `url` 或 `b64_json`。 |
| `image` | array<string> | 否 | 图片数组。`generations` 中所有图片都作为参考图；`edits` 中 `image[0]` 是底图，其余为附加参考图。 |

`/v1/images/edits` 支持 multipart/form-data；如果已经使用 JSON 请求体，原有调用方式保持不变。

### 结果语义

提交响应示例：

```json
{
  "taskOrderId": 2041888888888888888,
  "requestId": "req_20260407123000123456",
  "status": "submitted",
  "billingStatus": "pending",
  "progress": 20,
  "createTime": "2026-04-07T12:30:00"
}
```

任务详情示例：

```json
{
  "taskOrderId": 2041888888888888888,
  "requestId": "req_20260407123000123456",
  "modelCode": "nanobananapro",
  "modelName": "全能图片pro",
  "status": "succeeded",
  "billingStatus": "billed",
  "progress": 100,
  "resultCount": 1,
  "result": {
    "items": [
      {
        "url": "https://cdn.example.com/2026/04/07/demo.png",
        "type": "image"
      }
    ]
  }
}
```

## 尺寸映射

### 标准 10 种比例

`nanobanana` 与 `nanobananapro` 只支持下列标准 10 种比例。`nanobanana2` 也支持这些比例。

| 规格 | 比例 | 输出档位 | size | quality |
|---|---|---|---|---|
| 21:9 / 1K | 21:9 | 1K | `1584x672` | `standard` |
| 16:9 / 1K | 16:9 | 1K | `1376x768` | `standard` |
| 3:2 / 1K | 3:2 | 1K | `1264x848` | `standard` |
| 4:3 / 1K | 4:3 | 1K | `1200x896` | `standard` |
| 5:4 / 1K | 5:4 | 1K | `1152x928` | `standard` |
| 1:1 / 1K | 1:1 | 1K | `1024x1024` | `standard` |
| 4:5 / 1K | 4:5 | 1K | `928x1152` | `standard` |
| 3:4 / 1K | 3:4 | 1K | `896x1200` | `standard` |
| 2:3 / 1K | 2:3 | 1K | `848x1264` | `standard` |
| 9:16 / 1K | 9:16 | 1K | `768x1376` | `standard` |
| 21:9 / 2K | 21:9 | 2K | `3168x1344` | `hd` |
| 16:9 / 2K | 16:9 | 2K | `2752x1536` | `hd` |
| 3:2 / 2K | 3:2 | 2K | `2528x1696` | `hd` |
| 4:3 / 2K | 4:3 | 2K | `2400x1792` | `hd` |
| 5:4 / 2K | 5:4 | 2K | `2304x1856` | `hd` |
| 1:1 / 2K | 1:1 | 2K | `2048x2048` | `hd` |
| 4:5 / 2K | 4:5 | 2K | `1856x2304` | `hd` |
| 3:4 / 2K | 3:4 | 2K | `1792x2400` | `hd` |
| 2:3 / 2K | 2:3 | 2K | `1696x2528` | `hd` |
| 9:16 / 2K | 9:16 | 2K | `1536x2752` | `hd` |
| 21:9 / 4K | 21:9 | 4K | `6336x2688` | `hd` |
| 16:9 / 4K | 16:9 | 4K | `5504x3072` | `hd` |
| 3:2 / 4K | 3:2 | 4K | `5056x3392` | `hd` |
| 4:3 / 4K | 4:3 | 4K | `4800x3584` | `hd` |
| 5:4 / 4K | 5:4 | 4K | `4608x3712` | `hd` |
| 1:1 / 4K | 1:1 | 4K | `4096x4096` | `hd` |
| 4:5 / 4K | 4:5 | 4K | `3712x4608` | `hd` |
| 3:4 / 4K | 3:4 | 4K | `3584x4800` | `hd` |
| 2:3 / 4K | 2:3 | 4K | `3392x5056` | `hd` |
| 9:16 / 4K | 9:16 | 4K | `3072x5504` | `hd` |

### nanobanana2 扩展比例

`nanobanana2` 额外支持 `8:1`、`4:1`、`1:4`、`1:8`。

| 规格 | 比例 | 输出档位 | size | quality |
|---|---|---|---|---|
| 8:1 / 1K | 8:1 | 1K | `3072x384` | `standard` |
| 4:1 / 1K | 4:1 | 1K | `2048x512` | `standard` |
| 1:4 / 1K | 1:4 | 1K | `512x2048` | `standard` |
| 1:8 / 1K | 1:8 | 1K | `384x3072` | `standard` |
| 8:1 / 2K | 8:1 | 2K | `6144x768` | `hd` |
| 4:1 / 2K | 4:1 | 2K | `4096x1024` | `hd` |
| 1:4 / 2K | 1:4 | 2K | `1024x4096` | `hd` |
| 1:8 / 2K | 1:8 | 2K | `768x6144` | `hd` |
| 8:1 / 4K | 8:1 | 4K | `12288x1536` | `hd` |
| 4:1 / 4K | 4:1 | 4K | `8192x2048` | `hd` |
| 1:4 / 4K | 1:4 | 4K | `2048x8192` | `hd` |
| 1:8 / 4K | 1:8 | 4K | `1536x12288` | `hd` |

## 模型：nanobananapro

| 项目 | 内容 |
|---|---|
| 名称 | 全能图片pro |
| 模型代码 | `nanobananapro` |
| 能力类型 | 图片生成 |
| 参考输入 | 最多 6 张参考图 |
| 比例限制 | 只支持标准 10 种比例，不支持 `8:1`、`4:1`、`1:4`、`1:8` |

上游计费概览：

| 档位 | 上游价格 |
|---|---:|
| 1K | 0.04 积分/次 |
| 2K | 0.06 积分/次 |
| 4K | 0.10 积分/次 |

本系统默认价格按上游价格翻倍：

| 价格 key | 默认价格 |
|---|---:|
| `nanobananapro@1k` | 0.08 |
| `nanobananapro@2k` | 0.12 |
| `nanobananapro@4k` | 0.20 |

生成示例：

```bash
curl --request POST "https://aiapipro.vip/v1/images/generations" \
  --header "Content-Type: application/json" \
  --header "Authorization: Bearer sk_your_access_key" \
  --data '{
    "model": "nanobananapro",
    "prompt": "一只未来感狐狸站在霓虹街头，电影级光影，细节丰富",
    "n": 1,
    "size": "1584x672",
    "quality": "standard",
    "response_format": "url",
    "image": ["https://example.com/source-image.png", "https://example.com/reference-image-2.png"]
  }'
```

## 模型：nanobanana2

| 项目 | 内容 |
|---|---|
| 名称 | 全能图片2 |
| 模型代码 | `nanobanana2` |
| 能力类型 | 图片生成 |
| 参考输入 | 最多 6 张参考图 |
| 比例能力 | 支持标准 10 种比例和扩展比例 `8:1`、`4:1`、`1:4`、`1:8` |

上游计费概览：

| 档位 | 上游价格 |
|---|---:|
| 1K | 0.03 积分/次 |
| 2K | 0.05 积分/次 |
| 4K | 0.07 积分/次 |

本系统默认价格按上游价格翻倍：

| 价格 key | 默认价格 |
|---|---:|
| `nanobanana2@1k` | 0.06 |
| `nanobanana2@2k` | 0.10 |
| `nanobanana2@4k` | 0.14 |

生成示例：

```bash
curl --request POST "https://aiapipro.vip/v1/images/generations" \
  --header "Content-Type: application/json" \
  --header "Authorization: Bearer sk_your_access_key" \
  --data '{
    "model": "nanobanana2",
    "prompt": "一只未来感狐狸站在霓虹街头，电影级光影，细节丰富",
    "n": 1,
    "size": "1584x672",
    "quality": "standard",
    "response_format": "url",
    "image": ["https://example.com/source-image.png", "https://example.com/reference-image-2.png"]
  }'
```

## 模型：nanobanana

| 项目 | 内容 |
|---|---|
| 名称 | 全能图片 |
| 模型代码 | `nanobanana` |
| 能力类型 | 图片生成 |
| 参考输入 | 最多 4 张参考图 |
| 比例限制 | 标准 10 种比例 |

上游计费概览：

| 档位 | 上游价格 |
|---|---:|
| 1K | 0.02 积分/次 |
| 2K | 0.04 积分/次 |
| 4K | 0.06 积分/次 |

本系统默认价格按上游价格翻倍：

| 价格 key | 默认价格 |
|---|---:|
| `nanobanana@1k` | 0.04 |
| `nanobanana@2k` | 0.08 |
| `nanobanana@4k` | 0.12 |

生成示例：

```bash
curl --request POST "https://aiapipro.vip/v1/images/generations" \
  --header "Content-Type: application/json" \
  --header "Authorization: Bearer sk_your_access_key" \
  --data '{
    "model": "nanobanana",
    "prompt": "一只未来感狐狸站在霓虹街头，电影级光影，细节丰富",
    "n": 1,
    "size": "1584x672",
    "quality": "standard",
    "response_format": "url",
    "image": ["https://example.com/source-image.png", "https://example.com/reference-image-2.png"]
  }'
```

## 模型：gpt-image-2

| 项目 | 内容 |
|---|---|
| 名称 | 全能图片g2 |
| 模型代码 | `gpt-image-2` |
| 能力类型 | 图片生成 |
| 参考输入 | 最多 6 张参考图 |
| 尺寸规则 | 自定义 `WIDTHxHEIGHT`，例如 `1024x1024`、`1536x1024`、`2048x2048`、`3840x2160` |
| quality | `low`、`medium`、`high`，省略时默认 `low` |
| PSD | 支持 `output_psd: true`，结果会包含图片和 `document` 类型文件 |
| 图片字段 | `image`、`mask`、`reference_images` 支持公网 URL、data URI、纯 base64；multipart 可直接上传文件 |

上游计费概览：

| 档位 | 上游价格 |
|---|---:|
| LOW | 0.05 积分/次 |
| MEDIUM | 0.055 积分/次 |
| HIGH | 0.075 积分/次 |
| LOW + PSD | 0.08 积分/次 |
| MEDIUM + PSD | 0.085 积分/次 |
| HIGH + PSD | 0.10 积分/次 |
| PSD | 0.15 积分/次 |

本系统默认价格按上游价格翻倍：

| 价格 key | 默认价格 |
|---|---:|
| `gpt-image-2@low` | 0.10 |
| `gpt-image-2@medium` | 0.11 |
| `gpt-image-2@high` | 0.15 |
| `gpt-image-2@low@psd` | 0.16 |
| `gpt-image-2@medium@psd` | 0.17 |
| `gpt-image-2@high@psd` | 0.20 |
| `gpt-image-2@psd` | 0.30 |

生成示例：

```bash
curl --request POST "https://aiapipro.vip/v1/images/generations" \
  --header "Content-Type: application/json" \
  --header "Authorization: Bearer sk_your_access_key" \
  --data '{
    "model": "gpt-image-2",
    "prompt": "一只未来感狐狸站在霓虹街头，电影级光影，细节丰富",
    "n": 1,
    "size": "1024x1024",
    "quality": "low",
    "response_format": "url",
    "output_psd": true,
    "image": ["https://example.com/source-image.png", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA..."]
  }'
```

编辑示例：

```bash
curl --request POST "https://aiapipro.vip/v1/images/edits" \
  --header "Content-Type: application/json" \
  --header "Authorization: Bearer sk_your_access_key" \
  --data '{
    "model": "gpt-image-2",
    "prompt": "保持主体构图不变，把背景改成夜晚下雨的霓虹街道",
    "image": ["https://example.com/source-image.png", "https://example.com/reference-image-2.png"],
    "mask": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA...",
    "reference_images": ["https://example.com/source-image.png"],
    "size": "1024x1024",
    "quality": "low",
    "response_format": "url"
  }'
```

multipart 编辑示例：

```bash
curl --request POST "https://aiapipro.vip/v1/images/edits" \
  --header "Authorization: Bearer sk_your_access_key" \
  --form "model=gpt-image-2" \
  --form "prompt=保持主体构图不变，把背景改成夜晚下雨的霓虹街道" \
  --form "image=@./base.png" \
  --form "image=@./reference-extra.png" \
  --form "size=1024x1024" \
  --form "quality=low" \
  --form "response_format=url" \
  --form "mask=@./mask.png" \
  --form "reference_images=@./reference-1.png" \
  --form "reference_images=@./reference-2.png"
```

## 适配器开发要求

- 写或改 AIAPIPro 图片模型适配前，先核对本文档。
- `size` 必须按官方 OpenAI 兼容像素尺寸或 gpt-image-2 自定义 `WIDTHxHEIGHT` 处理。
- 不要在文档或适配器里推荐 `9x16-4k` 这种合并写法；如果为历史兼容支持，也必须在发送上游前转换为官方 `size`。
- Nano 系列的 `quality` 应根据尺寸档位传 `standard` 或 `hd`；gpt-image-2 使用 `low`、`medium`、`high`。
- `images/generations` 和 `images/edits` 都要支持 `image` 数组；`edits` 中 `image[0]` 是底图。
- 结果返回给用户前必须隐藏上游 `modelName`，并转存结果文件到 OSS。
