# Go 语言测试网关开发计划

这是一个用于构建一个具备请求记录、回放和Web管理界面的HTTP测试网关的开发计划。

## 1. 项目目标

开发一个HTTP代理服务器，该服务器能够：
1.  监听指定端口，并将所有流量转发到指定的目标URL。
2.  完整记录每个请求和响应的详细信息（URL、方法、头、体等）到SQLite数据库。
3.  提供一个Web管理界面（运行在独立的端口上），用于：
    *   用户登录和退出。
    *   查看已记录的请求列表。
    *   查看单个请求的完整详情。
    *   支持对请求进行修改和“重放”。

## 2. 技术选型

*   **后端语言**: Go (Golang)
*   **数据库**: SQLite3 (使用 `database/sql` 和 `github.com/mattn/go-sqlite3` 驱动)
*   **HTTP 服务**: Go 标准库 `net/http` 和 `net/http/httputil`
*   **Web 路由**: Go 标准库 `http.ServeMux` 或轻量级路由库 (如 `gorilla/mux`)
*   **前端**: 原生 HTML, CSS, JavaScript (无需复杂框架，以功能实现为先)

## 3. 开发阶段

---

### **阶段一：核心代理与日志记录功能**

**目标**: 实现一个可以工作的反向代理，并将请求和响应的核心信息打印到控制台。

1.  **项目初始化**:
    *   创建项目目录 `dGateway`。
    *   初始化 Go Module: `go mod init dgateway`

2.  **命令行参数处理**:
    *   使用 `flag` 包接收两个启动参数：
        *   `-port`: 监听端口 (例如: `8080`)
        *   `-target`: 目标服务器地址 (例如: `http://api.example.com`)

3.  **实现反向代理**:
    *   使用 `net/http/httputil.NewSingleHostReverseProxy` 创建一个代理处理器。这是实现转发最直接的方式。
    *   创建一个自定义的 Handler，它包裹 `ReverseProxy`，以便在请求转发前后执行我们的逻辑。

4.  **请求/响应信息捕获**:
    *   **请求捕获**: 在自定义 Handler 中，读取请求的 Method, URL, Headers 和 Body。
        *   **注意**: `http.Request.Body` 是一个 `io.ReadCloser`，只能读取一次。需要先读取内容，然后用读取到的字节重新创建一个 `io.ReadCloser` 赋值回 `request.Body`，以供后续的代理逻辑使用。
    *   **响应捕获**: 利用 `ReverseProxy` 的 `ModifyResponse` 钩子函数。在这个函数里，我们可以访问到从目标服务器返回的 `http.Response` 对象。
    *   读取响应的 Status Code, Headers 和 Body。同样，读取完 Body 后需要重新构建它，以便返回给原始客户端。
    *   将捕获到的所有信息（请求+响应）结构化并打印到标准输出。

---

### **阶段二：数据库集成**

**目标**: 将捕获到的请求/响应数据存入 SQLite 数据库。

1.  **数据库设计**:
    *   创建一个 `requests` 表，包含以下字段：
        *   `id` (INTEGER, PRIMARY KEY, AUTOINCREMENT)
        *   `timestamp` (DATETIME)
        *   `method` (TEXT)
        *   `url` (TEXT)
        *   `request_headers` (TEXT, 存储为 JSON 格式)
        *   `request_body` (BLOB)
        *   `status_code` (INTEGER)
        *   `response_headers` (TEXT, 存储为 JSON 格式)
        *   `response_body` (BLOB)

2.  **数据库模块 (`database.go`)**:
    *   编写 `InitDB()` 函数，用于连接数据库并执行 `CREATE TABLE`（如果表不存在）。
    *   编写 `LogRequest()` 函数，接收一个包含所有请求/响应信息的结构体，并将其插入数据库。

3.  **集成到代理**:
    *   在阶段一的自定义 Handler 中，替换掉打印到控制台的逻辑。
    *   在 `ModifyResponse` 钩子函数中，当所有信息（请求和响应）都捕获完毕后，调用 `LogRequest()` 函数将数据写入数据库。

---

### **阶段三：Web 管理界面**

**目标**: 构建一个用于查看和重放请求的 Web 应用。

1.  **启动第二个 Web 服务**:
    *   在 `main` 函数中，使用 goroutine 启动一个新的 HTTP 服务，监听在 `代理端口 + 1`。

2.  **API 端点设计**:
    *   `POST /login`: 处理登录请求。初期可使用硬编码的用户名/密码。成功后设置一个 session cookie。
    *   `POST /logout`: 清除 session cookie。
    *   `GET /api/requests`: (需认证) 从数据库查询并返回请求列表（可分页）。
    *   `GET /api/requests/{id}`: (需认证) 根据 ID 查询并返回单个请求的完整详情。
    *   `POST /api/replay`: (需认证) 接收一个包含（可能被修改过的）请求参数的 JSON 对象，执行该 HTTP 请求，并将结果直接返回给前端。

3.  **认证中间件**:
    *   编写一个 HTTP 中间件，用于检查受保护的 API 端点（如 `/api/*`）的请求是否带有有效的 session cookie。

4.  **前端页面 (`/static` 目录)**:
    *   **`login.html`**: 登录表单。
    *   **`index.html`**: 主页面。
        *   使用 JavaScript (`fetch`) 调用 `/api/requests` 获取数据，并动态渲染成一个表格。
        *   表格行点击后，调用 `/api/requests/{id}` 获取详情，并显示在一个模态框或详情区域。
        *   详情视图中包含一个“重放”按钮。
    *   **重放功能**:
        *   点击“重放”后，弹出一个模态框，其中包含可编辑的表单（URL, Method, Headers, Body）。
        *   用户修改后点击“执行”，JavaScript 将这些数据 POST 到 `/api/replay` 端点。
        *   将 `/api/replay` 返回的结果展示给用户。

---

### **阶段四：完善与打包**

**目标**: 提升应用的健壮性和易用性。

1.  **完善错误处理**: 在所有 I/O 操作、数据库操作和 HTTP 请求中添加健壮的错误处理和日志记录。
2.  **UI/UX 优化**: 优化管理界面的样式和交互，使其更易于使用。
3.  **构建与部署**:
    *   编写 `README.md`，说明如何编译和运行程序。
    *   提供二进制构建命令: `go build -o dgateway .`
    *   提供运行示例: `./dgateway -port=8080 -target="http://localhost:3000"`
4.  **代码结构优化**: 将不同的功能模块（如 `proxy`, `database`, `admin`)组织到不同的 Go 包中，使项目结构更清晰。
