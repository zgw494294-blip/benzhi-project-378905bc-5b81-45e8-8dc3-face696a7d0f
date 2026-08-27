# 方言田野语料证据审核与引用凭据

本项目为语言学田野团队提供浏览器工作台，管理方言录音元数据、转写证据、专家复核、冻结清单和可验证引用凭据。服务只处理元数据、片段摘要和小型示例，不分发大文件。

## 构建

```bash
go build ./cmd/dialectarchive
```

## 运行

```bash
go run ./cmd/dialectarchive -addr=127.0.0.1:19081
```

也可通过 `PORT` 指定端口，例如 `PORT=19082 go run ./cmd/dialectarchive`。浏览器访问 `http://127.0.0.1:19081/`。

## 测试

```bash
go test ./...
go run ./cmd/dialectarchive -addr=127.0.0.1:19081 -selfcheck
```

数据默认保存到 `.dialectarchive`，可用 `DIALECTARCHIVE_DATA` 指定目录。

## 主要接口

- `PATCH /api/batches/{id}`：携带 `expectedVersion` 修订批次资料，支持 `idempotencyKey`。
- `POST /api/segments`：登记片段并检查批次内重复摘要、说话人时间轴冲突和同意政策。
- `POST /api/batches/{id}/quality-check`：按批次重做质量检查；`POST /api/batches/{id}/preflight` 用于冻结前就绪度预检。
- `POST /api/annotations`：提交连续 revision 的转写；`GET /api/annotations?segment_id=...` 返回历史和 `current_revision`。
- `POST /api/releases`：冻结并签发凭据，可选 `expires_at`；`GET /api/manifests?manifest_sha256=...` 查询不可变清单，`/api/credentials/verify` 返回诊断原因码。
