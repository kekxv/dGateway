# dGateway - 基于 Go 的测试网关

dGateway 是一个简单的 HTTP/HTTPS 代理，它将所有传入的请求及其对应的响应记录到 SQLite 数据库中。它还提供了一个基于 Web 的管理面板，用于查看、检查和重放这些记录的请求。

## 功能特性

*   **HTTP 代理**: 将指定监听端口的所有请求转发到目标 URL。
*   **请求/响应记录**: 捕获 HTTP 请求和响应的完整详细信息（头部、正文、方法、URL、状态码）并存储在 SQLite 数据库中。在存储前自动解压缩 `gzip` 编码的正文。
*   **Web 管理面板**: 
    *   运行在独立端口上（代理端口 + 1）。
    *   登录/登出功能。
    *   列出所有记录的请求。
    *   允许查看每个请求的详细信息。
    *   支持重放请求并修改参数（方法、URL、头部、正文）。
    *   **多语言支持**: 管理面板支持多种语言（默认为英文和中文）。语言文件位于 `static/i18n/` 目录。
    *   **HAR 导出**: 以 HAR (HTTP Archive) 格式导出记录的请求，以便在其他工具中进行分析。

## 快速开始

### 先决条件

*   Go (版本 1.16 或更高)

### 构建应用程序

导航到项目根目录并运行：

```bash
go mod tidy
go build -o dgateway .
```

这将在当前目录中创建一个名为 `dgateway` 的可执行文件。

### 生成证书

要生成 HTTPS 支持所需的证书，请运行：

```bash
./dgateway -gen-certs
```

这将在 `certs/` 目录中生成 CA 证书和服务器证书。

### 运行应用程序

要启动 dGateway，您需要指定代理监听端口和目标 URL。您也可以选择指定 SQLite 数据库文件路径。

```bash
./dgateway -port=8080 -target="http://localhost:3000" -db="requests.db"
```

*   `-port`: 代理服务器监听传入请求的端口（例如 `8080`）。
*   `-target`: 要转发请求的目标服务器的完整 URL（例如 `http://localhost:3000`）。
*   `-db`: (可选) SQLite 数据库文件的路径。如果未提供，默认为当前目录中的 `requests.db`。
*   `-enable-https`: (可选) 在同一端口上启用 HTTPS 支持。需要先生成证书。

**HTTPS 支持:**
要启用 HTTPS 支持，请使用 `-enable-https` 标志。这允许代理在 `-port` 参数指定的同一端口上处理 HTTPS 请求。请注意，客户端必须明确使用 HTTPS 连接才能利用此功能。

**管理员凭据 (环境变量):**

默认情况下，管理员用户名是 `admin`，密码是 `admin`。您可以使用环境变量覆盖这些设置：

*   `ADMIN_USERNAME`: 设置管理面板的用户名。
*   `ADMIN_PASSWORD`: 设置管理面板的密码。

示例：
```bash
ADMIN_USERNAME=myuser ADMIN_PASSWORD=mypassword ./dgateway -port=8080 -target="http://localhost:3000"
```

启动后，您将看到显示代理和管理服务器监听端口的日志消息。

### 访问管理面板

管理面板将在 `http://localhost:<代理端口 + 1>` 上可用。例如，如果您的代理端口是 `8080`，管理面板将在 `http://localhost:8081`。

**默认登录凭据:**
*   **用户名**: `admin`
*   **密码**: `admin` (或通过环境变量设置的密码)

## 使用方法

1.  **代理请求**: 配置您的客户端（例如浏览器、API 客户端）将请求发送到 dGateway 的代理端口（例如 `localhost:8080`）。这些请求将被转发到您指定的目标，并且它们的详细信息将被记录。
2.  **查看日志**: 在浏览器中打开管理面板，登录后您将看到所有记录的请求列表。
3.  **检查详情**: 点击列表中的任何请求以查看其完整详细信息，包括请求头部、正文、响应头部和响应正文。解压缩的正文将被显示。
4.  **重放请求**: 从请求详情视图中，您可以点击"重放请求"按钮。这将打开一个表单，您可以在其中修改请求的方法、URL、头部和正文，然后再次发送。重放请求的响应将被显示。
5.  **导出 HAR**: 从主管理面板中，点击"导出 HAR"按钮，以 HAR (HTTP Archive) 格式下载所有记录的请求。此文件可用于 Chrome DevTools 或其他 HAR 分析工具中进行进一步分析。

## 项目结构

```
.dGateway/
├── main.go             # 主应用程序逻辑、代理和管理服务器设置
├── database.go         # 数据库初始化和记录功能
├── har_export.go       # HAR 导出功能
├── static/             # 前端静态文件 (HTML, CSS, JS)
│   ├── index.html      # 主管理仪表板页面
│   └── login.html      # 登录页面
│   └── i18n/           # 国际化文件
│       ├── en-US.json  # 英文语言包
│       └── zh-CN.json  # 中文语言包
├── certs/              # SSL/TLS 证书 (生成的)
│   ├── ca.crt          # CA 证书
│   ├── ca.key          # CA 私钥
│   ├── server.crt      # 服务器证书
│   └── server.key      # 服务器私钥
├── go.mod
├── go.sum
├── Makefile            # 构建和运行脚本
├── .gitignore          # Git 忽略文件
└── README.md           # 本文件
```

## 未来增强功能 (计划中)

*   更强大的认证和用户管理。
*   记录请求的过滤和搜索功能。
*   改进管理面板的 UI/UX。