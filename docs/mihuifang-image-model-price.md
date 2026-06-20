# Mihuifang 异步图片模型价格配置

下面这份 JSON 适配当前系统的 `ModelPrice` 配置，可以直接粘贴到后台：

```text
后台 -> 系统设置 -> 模型设置 / 计费设置 -> 模型价格 / Model prices
```

计价规则：

- 已在你给的成本价基础上统一加价 50%。
- `gpt-image-2` 按 `模型@清晰度@质量` 配置。
- `nano-banana`、`nano-banana2`、`nano-banana-pro` 当前系统按 `模型@清晰度` 配置。
- 当前系统不按 `ratio` 单独计费，所以同一模型同一清晰度下存在多个比例价格时，按该清晰度的最高成本价加 50% 配置，避免低估成本。
- `n` 不需要单独配置，系统会自动按生成张数倍乘。
- 如果使用渠道模型映射，价格 key 必须使用用户请求的模型名，不是上游模型名。

## 可直接使用的 ModelPrice JSON

```json
{
  "gemini-3-pro-image-preview": 0.2,
  "gemini-3.1-flash-image": 0.1333,

  "gemini-2.5-flash-image": 0.05,
  "gemini-2.5-flash-image@1k": 0.05,
  "gemini-2.5-flash-image@2k": 0.1,
  "gemini-2.5-flash-image@4k": 0.15,

  "gemini-3-pro-image": 0.1,
  "gemini-3-pro-image@1k": 0.1,
  "gemini-3-pro-image@2k": 0.15,
  "gemini-3-pro-image@4k": 0.2,

  "gpt-image-2": 0.06,
  "gpt-image-2@1k@low": 0.06,
  "gpt-image-2@2k@low": 0.1,
  "gpt-image-2@4k@low": 0.15,
  "gpt-image-2@1k@medium": 0.06,
  "gpt-image-2@2k@medium": 0.1,
  "gpt-image-2@4k@medium": 0.15,
  "gpt-image-2@1k@high": 0.1,
  "gpt-image-2@2k@high": 0.16,
  "gpt-image-2@4k@high": 0.3
}
```

## 如果你配置了模型映射

例如渠道里这样映射：

```json
{
  "gemini-3-pro-image": "nano-banana-pro"
}
```

那么价格必须按用户请求模型名 `gemini-3-pro-image` 配，而不是按 `nano-banana-pro` 配：

```json
{
  "gemini-3-pro-image": 0.1,
  "gemini-3-pro-image@1k": 0.1,
  "gemini-3-pro-image@2k": 0.15,
  "gemini-3-pro-image@4k": 0.2
}
```

如果只配置 `nano-banana-pro@4k`，但用户调用 `gemini-3-pro-image`，系统会找不到价格并报错。

## 请求参数和价格 key 对应关系

### gpt-image-2

请求示例：

```json
{
  "model": "gpt-image-2",
  "prompt": "一张产品海报",
  "resolution": "4K",
  "quality": "high",
  "n": 2
}
```

命中的价格 key：

```text
gpt-image-2@4k@high
```

计费：

```text
0.3 * 2 = 0.6
```

### Nano Banana 系列

请求示例：

```json
{
  "model": "nano-banana-pro",
  "prompt": "一张产品海报",
  "resolution": "2K",
  "aspect_ratio": "16:9",
  "n": 3
}
```

命中的价格 key：

```text
nano-banana-pro@2k
```

计费：

```text
0.15 * 3 = 0.45
```

## 常见错误

如果返回：

```json
{
  "message": "model price is required for gemini-3-pro-image@4k"
}
```

说明当前用户请求模型是 `gemini-3-pro-image`，并且清晰度被识别为 `4k`，但 `ModelPrice` 里没有配置：

```json
{
  "gemini-3-pro-image@4k": 0.2
}
```

补上对应 key 后保存即可。
