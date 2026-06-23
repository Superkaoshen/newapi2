# Mihuifang / AIAPIPro 图片模型调用文档

本文是旧 `mihuifang` 图片模型文档的归档入口。当前实现已经按 AIAPIPro 图片模型重新整理，实际对外调用请查看 [AIAPIPro 异步图片调用文档](./aiapipro-image-api-call-guide.md)，适配器开发请先查看 [AIAPIPro 图片模型官方参考文档](./aiapipro-image-models-reference.md)。

## 当前接口

| 用途 | 方法 | 路径 |
|---|---|---|
| 文生图 | `POST` | `/v1/images/generations` |
| 图生图 / 图片编辑 | `POST` | `/v1/images/edits` |
| 查询任务 | `GET` | `/v1/tasks/{requestId}` |

旧版 `/v1/async/generations` 仍作为兼容入口保留，但新文档和新接入优先使用上表接口。

## 当前模型

| 模型代码 | 名称 | 参考图限制 | 尺寸规则 |
|---|---|---:|---|
| `nanobanana` | 全能图片 | 4 张 | 标准 10 种官方像素尺寸 |
| `nanobanana2` | 全能图片2 | 6 张 | 标准 10 种官方像素尺寸，额外支持 `8:1`、`4:1`、`1:4`、`1:8` |
| `nanobananapro` | 全能图片pro | 6 张 | 标准 10 种官方像素尺寸，不支持扩展比例 |
| `gpt-image-2` | 全能图片g2 | 6 张 | 自定义 `WIDTHxHEIGHT`，支持 `low`、`medium`、`high` 和 `output_psd` |

## 重要兼容说明

- `size` 推荐直接传官方像素尺寸，例如 `2048x2048`、`1536x2752`、`5504x3072`。
- 历史的“比例 + 档位”写法只做兼容输入，适配器会在发送上游前转换为官方像素尺寸。
- Nano 系列发送上游时会按尺寸档位推导 `quality`：`1k` 为 `standard`，`2k` / `4k` 为 `hd`。
- `gpt-image-2` 的 `quality` 只支持 `low`、`medium`、`high`；省略时按 `low` 计费。
- 查询响应对用户隐藏上游 `modelName`，`modelCode` 返回用户请求的公开模型名。
- 渠道编辑里的“模型映射”可把公开模型名映射到上游模型码；公开模型名未配置价格时会回退到映射后的上游模型价格。
