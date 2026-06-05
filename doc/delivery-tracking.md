# iOS 发布程序交付追踪

## 交付步骤

| 步骤 | 状态 | 记录 |
| --- | --- | --- |
| 1. 初始化 Go 项目结构 | 已完成 | 已创建 Go 模块、后端目录、前端目录和运行时目录约定。 |
| 2. 后端核心能力 | 已完成 | 已实现配置读写、Basic Auth、IPA 上传、plist 生成、二维码生成。 |
| 3. 嵌入式前端页面 | 已完成 | 已实现 `/publish` 公开页与 `/internal` 管理页，前端资源通过 Go `embed` 内嵌。 |
| 4. 测试与构建 | 已完成 | `go test ./...` 通过，`go build ./cmd/iospublisher` 通过。 |
| 5. 交付验收记录 | 已完成 | 已记录功能、测试、构建结果和本地冒烟验证结果。 |

## 验收项

| 验收项 | 状态 | 记录 |
| --- | --- | --- |
| 单个 Go 二进制可构建 | 已通过 | 已生成 `iospublisher.exe`。 |
| `/publish` 可公开访问 | 已通过 | 本地冒烟验证返回 `200`。 |
| `/internal` 受 Basic Auth 保护 | 已通过 | 未认证返回 `401`，使用 `admin/admin` 返回 `200`。 |
| 可上传单个 IPA，默认限制 `2GB` | 已通过 | 上传接口限制默认值为 `2147483648` 字节，端到端测试覆盖小型 IPA 上传。 |
| 可修改展示名称、IPA 地址和 plist 地址 | 已通过 | `/api/config` 测试和前端管理页均已覆盖。 |
| 生成 plist 时填写 `bundleIdentifier` 和 `bundleVersion` | 已通过 | `/api/plist/generate` 测试覆盖生成逻辑。 |
| `/manifest.plist` 可公开访问 | 已通过 | 端到端测试覆盖生成后公开访问。 |
| `/qr.png` 可生成安装二维码 | 已通过 | 二维码 PNG 生成测试和端到端测试均已覆盖。 |
| 前端资源嵌入 Go 二进制 | 已通过 | `web` 包使用 `//go:embed` 嵌入 HTML、CSS、JS。 |
| `/publish` 不提供 plist 直接下载入口 | 已通过 | 公开页仅保留安装按钮、安装链接二维码，不展示 plist 下载按钮。 |
| 管理页生成 plist 后提供下载按钮 | 已通过 | `/internal` 的 plist 区域在检测到已生成 plist 后显示“下载 plist”按钮。 |
| 二维码使用内部短链 | 已通过 | 新增 `/install`，二维码编码短链地址，由服务端跳转到实际 `itms-services` 地址。 |
| 管理页可维护发布说明 | 已通过 | 配置中新增 `releaseNotes`，管理页保存后发布页展示。 |
| 生成 plist 时可配置 title | 已通过 | `/api/plist/generate` 新增 `title` 参数，写入 plist metadata title。 |
| 二进制同目录运行配置 | 已通过 | 新增同目录 `config.json`，可配置 IP、端口、Basic Auth、数据目录和最大上传大小。 |
| HTTPS 由部署代理负责 | 已确认 | 应用自身不内置 HTTPS。 |

## 验证记录

### 2026-05-27

- `go test ./...`：通过。
- `go build ./cmd/iospublisher`：通过。
- 本地服务冒烟：
  - `GET http://127.0.0.1:18080/publish` 返回 `200`。
  - `GET http://127.0.0.1:18080/internal` 未认证返回 `401`。
  - `GET http://127.0.0.1:18080/internal` 使用 `admin/admin` 返回 `200`。
  - `GET http://127.0.0.1:18080/api/publish` 返回默认发布状态，未上传 IPA 时 `ready=false`。

### 2026-05-27 调整记录

- 公开发布页移除 plist 直接下载入口。
- 管理页 plist 区域增加“下载 plist”按钮，并在 plist 已生成后显示。
- `GET /manifest.plist?download=1` 增加附件下载响应头。
- 新增 `/install` 短安装链接，公开页安装按钮和二维码均使用该短链。
- 新增发布说明配置，并在公开发布页展示。
- plist 生成接口新增 `title` 参数，与 `bundleIdentifier`、`bundleVersion` 一起填写。

### 2026-05-27 二次验证

- `go test ./...`：通过。
- `go build ./cmd/iospublisher`：通过。
- `GET http://127.0.0.1:8080/api/publish` 返回 `installUrl=http://127.0.0.1:8080/install`。
- `GET http://127.0.0.1:8080/install` 返回 `302`，跳转到实际 `itms-services` 地址。
- `/publish` 页面包含发布说明区域，且不包含 plist 下载入口。
- `/internal` 页面包含发布说明输入、plist title 输入和 plist 下载按钮。

### 2026-05-27 运行配置调整

- 新增二进制同目录 `config.json` 作为运行配置文件。
- 启动时如果 `config.json` 不存在，会自动生成默认配置。
- 配置项包括 `ip`、`port`、`dataDir`、`auth.user`、`auth.password`、`maxUploadBytes`。
- `dataDir` 为相对路径时按 `config.json` 所在目录解析。
- 环境变量仍可覆盖同名配置，便于部署临时调整。
- 本地验证已生成默认 `config.json`，并且 `GET http://127.0.0.1:8080/publish` 返回 `200`。

### 2026-05-27 R4S 打包

- 已交叉编译 Linux ARM64/aarch64 二进制。
- 已生成部署包 `dist/iospublisher-r4s-linux-arm64.tar.gz`。
- 部署包包含 `iospublisher`、`config.json` 和 `README.md`。
- 已验证二进制为 ELF 文件，适用于 Linux。
- 压缩包 SHA256：`139172B2E2A31E42ACA41E696AE63D8A1375F7CDC21BBE414E7EAD37EA3B4FE4`。

### 2026-05-27 Linux x86_64 打包

- 已交叉编译 Linux x86_64/amd64 二进制。
- 已生成部署包 `dist/iospublisher-linux-amd64.tar.gz`。
- 部署包包含 `iospublisher`、`config.json` 和 `README.md`。
- 已验证二进制为 ELF 文件，适用于 Linux。
- 压缩包 SHA256：`05629632072563271251FB1D0B4C75391EE657FF74A845A802920239AB9F7AFA`。

## 已实现文件

- `cmd/iospublisher/main.go`：程序入口、运行配置读取、环境变量覆盖、HTTP 服务启动。
- `internal/auth`：Basic Auth 中间件。
- `internal/config`：JSON 配置读写。
- `internal/runtimeconfig`：二进制同目录运行配置读写。
- `internal/plist`：iOS OTA manifest plist 生成。
- `internal/qrcode`：离线二维码 PNG 生成。
- `internal/server`：路由、上传、文件访问和 API。
- `web`：嵌入式前端页面与静态资源。

## 运行约定

- 默认运行配置文件：二进制同目录 `config.json`。
- 默认监听 IP：`0.0.0.0`。
- 默认监听端口：`8080`。
- 默认管理账号：`admin`。
- 默认管理密码：`admin`。
- 默认数据目录：`data`，相对路径按 `config.json` 所在目录解析。
- 默认上传限制：`2147483648` 字节，即 `2GB`。
- `config.json` 示例：

```json
{
  "ip": "0.0.0.0",
  "port": 8080,
  "dataDir": "data",
  "auth": {
    "user": "admin",
    "password": "admin"
  },
  "maxUploadBytes": 2147483648
}
```

- 可通过环境变量覆盖：
  - `IOSPUB_CONFIG_PATH`
  - `IOSPUB_IP`
  - `IOSPUB_PORT`
  - `IOSPUB_ADDR`
  - `IOSPUB_ADMIN_USER`
  - `IOSPUB_ADMIN_PASSWORD`
  - `IOSPUB_DATA_DIR`
  - `IOSPUB_MAX_UPLOAD_BYTES`
