# DoubaoProxy

DoubaoProxy 是一个使用 Go 编写的豆包非官方代理服务，复刻自 [DoubaoFreeApi](https://github.com/XilyFeAAAA/DoubaoFreeApi) 的 Python 实现。项目提供统一的 HTTP API，便于在本地或自建环境中访问豆包聊天与文件上传能力。

## 功能特性

- **聊天补全**：兼容 SSE 流式输出，支持文字与图片消息。
- **上下文会话**：自动管理 conversation_id/section_id，与会话池联动。
- **会话池管理**：同时支持游客与登录账号 Session，按需随机分配。
- **文件上传**：实现 prepare → apply → upload → commit 的四步上传流程，内置 AWS SigV4 签名。
- **可配置化**：核心超时、监听地址与 Session 文件路径可通过环境变量调整。

## 目录结构

```
.
├── main.go                   // 程序入口，装配配置、会话池、服务与路由
├── session.example.json      // Session 配置示例
├── session.json              // 运行时使用的 Session 配置（需自行填写）
├── internal/
│   ├── config/               // 环境变量配置解析
│   ├── handler/              // gin 路由与请求处理
│   ├── model/                // 请求/响应结构体与错误类型
│   ├── server/               // HTTP Server 封装与日志中间件
│   ├── session/              // 会话池管理（游客/登录账号）
│   └── service/
│       └── doubao/           // 豆包业务逻辑：聊天、删除、上传、SSE 解析
└── go.mod / go.sum           // Go 模块依赖
```

## 环境要求

- Go 1.22 及以上
- 可访问豆包网页版（用于抓取 Session）
- 运行环境需具备外网访问能力

## 获取 Session

1. 登录豆包网页版（建议 Chrome/Edge）。
2. 打开开发者工具，切换到 Network。
3. 发起任意一次聊天请求，记录以下字段：
   - `cookie`
   - `device_id`
   - `tea_uuid`
   - `web_id`
   - `room_id`
   - `x_flow_trace`
4. 将字段写入 `session.json`，格式参考 `session.example.json`。
   - 登录账号请将 `"guest": false`（或移除该字段）。
   - 游客账号需设置 `"guest": true`，且受配额限制。

> 使用 UTF-8（无 BOM）保存 `session.json`，避免解析报错。

## 可用环境变量

| 变量名 | 默认值 | 说明 |
| --- | --- | --- |
| `HTTP_ADDR` | `:8000` | HTTP 服务监听地址 |
| `SESSION_CONFIG` | `session.json` | Session 配置文件路径 |
| `SHUTDOWN_TIMEOUT_SEC` | `10` | 优雅关机等待秒数 |
| `HTTP_CLIENT_TIMEOUT_S` | `300` | 调用豆包接口的超时时间（秒） |
| `HTTP_READ_TIMEOUT_S` | `30` | 服务读取请求的超时（秒） |
| `HTTP_WRITE_TIMEOUT_S` | `30` | 服务写响应的超时（秒） |

在 PowerShell 中设置示例：

```powershell
$env:HTTP_CLIENT_TIMEOUT_S = "180"
$env:HTTP_READ_TIMEOUT_S   = "60"
$env:HTTP_WRITE_TIMEOUT_S  = "60"
```

## 构建与运行

```powershell
# 获取依赖
go mod tidy

# 编译
go build -o doubao-proxy .

# 运行（确保 session.json 已配置）
./doubao-proxy
```

或者直接使用 `go run .`：

```powershell
$env:SESSION_CONFIG = "session.json"
go run .
```

服务启动后默认监听 `http://localhost:8000`。

## API 说明

### 健康检查

```http
GET /healthz
```

### 聊天补全

```http
POST /api/chat/completions
Content-Type: application/json
```

请求示例：

```json
{
  "prompt": "你好，简单介绍一下自己？",
  "guest": false,
  "attachments": [],
  "conversation_id": "",
  "section_id": "",
  "use_deep_think": false,
  "use_auto_cot": false
}
```

响应示例：

```json
{
  "text": "我是豆包...",
  "img_urls": [],
  "conversation_id": "7098xxxxxxxxxxxxx",
  "messageg_id": "m-xxxxxxxx",
  "section_id": "s-xxxxxxxx"
}
```

后续请求若需保持上下文，传入上一次响应中的 `conversation_id` 与 `section_id`。

### 删除会话

```http
POST /api/chat/delete?conversation_id={id}
```

返回：

```json
{
  "ok": true,
  "msg": ""
}
```

### 文件上传

```http
POST /api/file/upload?file_type=2&file_name=test.png
Content-Type: application/octet-stream
```

Body 为文件二进制内容，返回值可直接放入聊天的 `attachments` 字段。

## 测试示例

PowerShell 下的简单调用：

```powershell
curl -X POST "http://localhost:8000/api/chat/completions" `
     -H "Content-Type: application/json" `
     -d '{
           "prompt": "你好，简单介绍一下自己？",
           "guest": false,
           "attachments": [],
           "conversation_id": "",
           "section_id": "",
           "use_deep_think": false,
           "use_auto_cot": false
         }'
```

若使用游客 Session，请将 `guest` 设为 `true`，且不要携带上下文 ID。

## 日志

- 默认输出 JSON 格式，例如：

  ```json
  {"time":"2025-10-14T16:26:04+08:00","level":"INFO","msg":"http request","method":"POST","path":"/api/chat/completions","status":200,"duration":"2m1.5s","ip":"127.0.0.1"}
  ```
- 超长 duration 代表豆包响应时间较长，属于正常情况。

## 许可

本项目沿用原仓库的 [MIT License](LICENSE)。
