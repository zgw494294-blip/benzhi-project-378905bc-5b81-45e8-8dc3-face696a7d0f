# BENZHI_README

基于 Go 实现的方言田野语料证据审核与引用凭据 Web 项目，一款后端服务，用于支持方言田野语料证据审核与引用凭据的核心业务流程。

## 项目说明
- 项目：benzhi-project-378905bc-5b81-45e8-8dc3-face696a7d0f
- 项目用途：用于支持方言田野语料证据审核与引用凭据的核心业务流程。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/dialectarchive -addr=127.0.0.1:19081 -selfcheck
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-378905bc-5b81-45e8-8dc3-face696a7d0f-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-378905bc-5b81-45e8-8dc3-face696a7d0f-arm64 linux/arm64
docker run -it benzhi-project-378905bc-5b81-45e8-8dc3-face696a7d0f-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/dialectarchive -addr=127.0.0.1:19081 -selfcheck`
