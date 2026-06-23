# Mihuifang / AIAPIPro 图片模型价格配置

本文仅保留当前系统价格摘要。完整模型字段、尺寸表、参考图限制和适配器要求请查看 [AIAPIPro 图片模型官方参考文档](./aiapipro-image-models-reference.md)，对外调用示例请查看 [AIAPIPro 异步图片调用文档](./aiapipro-image-api-call-guide.md)。

旧版 `mihuifang` 文档中的比例档位写法和加价规则已废弃；后续适配以 AIAPIPro 官方文档为准。

## 当前默认价格

本系统默认价格按上游价格翻倍。

| 价格 key | 默认价格 |
|---|---:|
| `gpt-image-2` | 0.10 |
| `gpt-image-2@low` | 0.10 |
| `gpt-image-2@medium` | 0.11 |
| `gpt-image-2@high` | 0.15 |
| `gpt-image-2@low@psd` | 0.16 |
| `gpt-image-2@medium@psd` | 0.17 |
| `gpt-image-2@high@psd` | 0.20 |
| `gpt-image-2@psd` | 0.30 |
| `nanobanana` | 0.04 |
| `nanobanana@1k` | 0.04 |
| `nanobanana@2k` | 0.08 |
| `nanobanana@4k` | 0.12 |
| `nanobanana2` | 0.06 |
| `nanobanana2@1k` | 0.06 |
| `nanobanana2@2k` | 0.10 |
| `nanobanana2@4k` | 0.14 |
| `nanobananapro` | 0.08 |
| `nanobananapro@1k` | 0.08 |
| `nanobananapro@2k` | 0.12 |
| `nanobananapro@4k` | 0.20 |

旧别名 `nano-banana`、`nano-banana2`、`nano-banana-pro` 也保留对应默认价格，分别映射到 `nanobanana`、`nanobanana2`、`nanobananapro`。

## 计费规则

- Nano 系列按官方像素 `size` 识别 `1k`、`2k`、`4k` 档位。
- `gpt-image-2` 按 `quality` 和 `output_psd` 识别价格 key，省略 `quality` 时按 `low`。
- `n` 会作为输出数量倍率参与计费；不传时按 `1`。
- 如果渠道模型映射使用公开模型名，公开模型名没有单独配置价格时，适配器会回退到映射后的上游模型价格。
