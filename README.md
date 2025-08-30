# dGateway - A Go-based Test Gateway

View Screenshots：[Screenshots](assets)

dGateway is a simple HTTP/HTTPS proxy that logs all incoming requests and their corresponding responses to an SQLite database. It also provides a web-based administration panel to view, inspect, and replay these recorded requests.

## Features

*   **HTTP Proxy**: Forwards all requests from a specified listening port to a target URL.
*   **Request/Response Logging**: Captures full details of HTTP requests and responses (headers, body, method, URL, status code) and stores them in an SQLite database. Automatically decompresses `gzip` encoded bodies before storing.
*   **Web Admin Panel**: 
    *   Runs on a separate port (proxy port + 1).
    *   Login/Logout functionality.
    *   Lists all recorded requests.
    *   Allows viewing detailed information for each request.
    *   Supports replaying requests with modifiable parameters (method, URL, headers, body).
    *   **Multi-language Support**: The admin panel supports multiple languages (English and Chinese by default). Language files are located in `static/i18n/`.
    *   **HAR Export**: Export recorded requests in HAR (HTTP Archive) format for analysis in other tools.

## Getting Started

### Prerequisites

*   Go (version 1.16 or higher)

### Build the Application

Navigate to the project root directory and run:

```bash
go mod tidy
go build -o dgateway .
```

This will create an executable named `dgateway` in your current directory.

### Generate Certificates

To generate the necessary certificates for HTTPS support, run:

```bash
./dgateway -gen-certs
```

This will generate the CA certificate and server certificate in the `certs/` directory.

### Run the Application

To start the dGateway, you need to specify the proxy listening port and the target URL. Optionally, you can specify the SQLite database file path.

```bash
./dgateway -port=8080 -target="http://localhost:3000" -db="requests.db"
```

*   `-port`: The port on which the proxy server will listen for incoming requests (e.g., `8080`).
*   `-target`: The full URL of the target server to which requests will be forwarded (e.g., `http://localhost:3000`).
*   `-db`: (Optional) The path to the SQLite database file. If not provided, it defaults to `requests.db` in the current directory.
*   `-enable-https`: (Optional) Enable HTTPS support on the same port. Requires certificates to be generated first.

**HTTPS Support:**
To enable HTTPS support, use the `-enable-https` flag. This allows the proxy to handle HTTPS requests on the same port specified by the `-port` parameter. Note that clients must explicitly connect using HTTPS to utilize this feature.

**Admin Credentials (Environment Variables):**

By default, the admin username is `admin` and the password is `admin`. You can override these using environment variables:

*   `ADMIN_USERNAME`: Sets the username for the admin panel.
*   `ADMIN_PASSWORD`: Sets the password for the admin panel.

Example:
```bash
ADMIN_USERNAME=myuser ADMIN_PASSWORD=mypassword ./dgateway -port=8080 -target="http://localhost:3000"
```

Once started, you will see log messages indicating the proxy and admin server listening ports.

### Accessing the Admin Panel

The admin panel will be available on `http://localhost:<proxy_port + 1>`. For example, if your proxy port is `8080`, the admin panel will be at `http://localhost:8081`.

**Default Login Credentials:**
*   **Username**: `admin`
*   **Password**: `admin` (or as set by environment variables)

## Usage

1.  **Proxy Requests**: Configure your client (e.g., browser, API client) to send requests to the dGateway's proxy port (e.g., `localhost:8080`). These requests will be forwarded to your specified target, and their details will be logged.
2.  **View Logs**: Open the admin panel in your browser, log in, and you will see a list of all recorded requests.
3.  **Inspect Details**: Click on any request in the list to view its full details, including request headers, body, response headers, and response body. Decompressed bodies will be displayed.
4.  **Replay Requests**: From the request details view, you can click the "Replay Request" button. This will open a form where you can modify the request's method, URL, headers, and body before sending it again. The response from the replayed request will be displayed.
5.  **Export HAR**: From the main admin panel, click the "Export HAR" button to download all recorded requests in HAR (HTTP Archive) format. This file can be used for further analysis in tools like Chrome DevTools or other HAR analyzers.

## Project Structure

```
.dGateway/
├── main.go             # Main application logic, proxy, and admin server setup
├── database.go         # Database initialization and logging functions
├── har_export.go       # HAR export functionality
├── static/             # Frontend static files (HTML, CSS, JS)
│   ├── index.html      # Main admin dashboard page
│   └── login.html      # Login page
│   └── i18n/           # Internationalization files
│       ├── en-US.json  # English language pack
│       └── zh-CN.json  # Chinese language pack
├── certs/              # SSL/TLS certificates (generated)
│   ├── ca.crt          # CA certificate
│   ├── ca.key          # CA private key
│   ├── server.crt      # Server certificate
│   └── server.key      # Server private key
├── go.mod
├── go.sum
├── Makefile            # Build and run scripts
├── .gitignore          # Git ignore file
└── README.md           # This file
```

## Future Enhancements (Planned)

*   More robust authentication and user management.
*   Filtering and searching capabilities for recorded requests.
*   Improved UI/UX for the admin panel.