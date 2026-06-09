# Vectorizer 异步矢量化 API

本文档描述本系统适配后的图片转矢量文件接口。调用方只访问本系统地址；上游任务 ID 不会暴露。任务成功后，系统会下载上游生成的 `svg` 或 `eps` 文件并上传到阿里云 OSS，最终返回 OSS 公网链接。

## 渠道配置

1. 新增渠道类型选择 `Vectorizer`。
2. 基础地址默认使用 `https://lanshan.shenzhuo.vip`，如需自定义只填写上游根地址，不要带 `/add_task`、`/try_get` 或 `/get_image`。
3. 模型列表配置 `vectorizer`。
4. 上游如果不需要鉴权，密钥可留空；填写密钥时系统会向上游发送 `Authorization: Bearer <key>`。
5. 必须在系统设置中启用并正确配置阿里云 OSS，否则任务完成后会按失败处理，不返回上游临时文件链接。

## 提交任务

`POST /v1/api/generate`

### Header

```http
Authorization: Bearer <系统令牌>
Content-Type: application/json
```

### Body

```json
{
  "model": "vectorizer",
  "image": "https://example.com/source.png",
  "format": "svg",
  "replyType": "async"
}
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 是 | 固定传 `vectorizer` |
| `image` | 是 | 输入图片，支持公网 URL 或 `data:image/...;base64,...` |
| `images` | 否 | 兼容数组写法；当 `image` 为空时取第一张 |
| `format` | 否 | 输出格式，支持 `svg`、`eps`，默认 `eps` |
| `replyType` | 否 | 支持 `async` 或 `json`，系统始终按异步任务处理 |

### Response

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "running"
}
```

返回的 `id` 是本系统公开任务 ID，用于查询结果。

## 查询任务

`GET /v1/api/result?id=<task_id>`

### Header

```http
Authorization: Bearer <系统令牌>
```

### 处理中

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "running",
  "progress": 30
}
```

### 成功

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "succeeded",
  "progress": 100,
  "results": [
    {
      "url": "https://oss.example.com/openai-images/2026/06/09/xxxx.svg"
    }
  ]
}
```

### 失败

```json
{
  "id": "task_xxxxxxxxxxxxx",
  "status": "failed",
  "progress": 100,
  "error": "task failed"
}
```

## 状态说明

| 状态 | 说明 |
| --- | --- |
| `running` | 任务仍在处理 |
| `succeeded` | 任务成功，`results[0].url` 为 OSS 文件链接 |
| `failed` | 任务失败，查看 `error` 获取原因 |

## 调用示例

```bash
curl -X POST "https://your-domain.com/v1/api/generate" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "vectorizer",
    "image": "https://example.com/logo.png",
    "format": "svg",
    "replyType": "async"
  }'
```

```bash
curl "https://your-domain.com/v1/api/result?id=task_xxxxxxxxxxxxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```
