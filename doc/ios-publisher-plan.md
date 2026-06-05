# 轻量 iOS 发布程序计划

## 目标

实现一个 Go 编写的轻量 iOS 发布程序，最终交付为单个可执行二进制文件。程序内嵌前端页面，仅支持发布一个 IPA 包，不支持多包管理，也不做历史版本管理。

## 功能范围

### 公开发布页

- 路由：`GET /publish`
- 无需登录。
- 展示当前应用名称、安装入口和必要的发布说明。
- 提供短安装链接，固定为 `/install`，由服务端跳转到实际 iOS OTA 安装地址。
- 展示安装二维码，二维码内容为 `/install` 的完整访问地址，避免二维码内容过长。
- 展示当前发布说明。
- 如果尚未上传 IPA 或尚未生成 plist，页面展示未配置状态。

### 内部管理页

- 路由：`GET /internal`
- 使用 Basic Auth 保护。
- 功能：
  - 上传 IPA 包。
  - 修改公开展示名称。
  - 修改公开发布说明。
  - 修改 plist 中使用的 IPA 下载地址。
  - 修改 plist 文件对外访问地址。
  - 生成或重新生成 plist，生成时填写 `bundleIdentifier`、`bundleVersion` 和 plist `title`。
  - 查看当前配置状态。

### 管理接口

- 所有管理接口均走 Basic Auth。
- 推荐接口：
  - `GET /api/state`：读取当前配置和文件状态。
  - `POST /api/upload`：上传 IPA。
  - `POST /api/config`：保存应用名称、发布说明、IPA 下载地址、plist 对外地址等配置。
  - `POST /api/plist/generate`：根据当前配置，以及本次提交的 `bundleIdentifier`、`bundleVersion` 和 `title` 生成 plist。

### 文件访问

- `GET /files/app.ipa`：下载当前 IPA。
- `GET /manifest.plist`：访问当前生成的 plist。
- `GET /install`：短安装链接，跳转到 `itms-services://?action=download-manifest&url=<plist_url>`。
- 以上地址默认公开访问，便于 iOS 设备安装。

## 不做的事情

- 不支持多个应用。
- 不支持多个版本。
- 不支持用户系统或复杂权限。
- 不解析 IPA 内部信息。
- 不接入数据库。
- 不提供证书、签名、重签名能力。

## 技术方案

### 后端

- 语言：Go。
- HTTP 框架：优先使用标准库 `net/http`，减少依赖，保持二进制轻量。
- 前端嵌入：使用 `embed` 包，通过 `//go:embed` 将静态资源嵌入二进制。
- 运行配置：二进制同目录使用 `config.json`，用于配置监听 IP、端口、Basic Auth 账号密码、数据目录和最大上传大小。
- 配置持久化：使用本地 JSON 文件，例如 `data/config.json`。
- 上传文件存储：默认保存到 `data/app.ipa`。
- 上传大小限制：默认最大 `2GB`，可通过环境变量调整。
- plist 输出：默认保存到 `data/manifest.plist`。
- 二维码输出：由服务端根据短安装链接 `/install` 生成 PNG，避免公开页依赖外部 CDN。

### 前端

- 使用原生 HTML/CSS/JavaScript 或极轻量构建方式。
- 页面作为静态资源嵌入 Go。
- 路由由 Go 返回对应页面：
  - `/publish` 返回公开发布页。
  - `/internal` 返回内部管理页。
- 前端通过 fetch 调用管理 API。

### 单二进制交付

- 前端源码在构建时嵌入 Go。
- 运行时只依赖一个二进制文件。
- 运行后会在工作目录创建或使用 `data` 目录保存上传文件和配置。

## 配置模型

建议配置结构：

```json
{
  "appName": "My iOS App",
  "releaseNotes": "本次发布说明",
  "ipaUrl": "https://example.com/files/app.ipa",
  "plistUrl": "https://example.com/manifest.plist",
  "updatedAt": "2026-05-27T00:00:00Z"
}
```

说明：

- `appName` 用于发布页展示。
- `releaseNotes` 用于发布页展示当前发布说明。
- `ipaUrl` 是 plist 中指向 IPA 的下载地址。
- `plistUrl` 是 `/install` 跳转到 `itms-services` 时使用的 manifest 地址。
- 如用户没有手动填写 `ipaUrl` 或 `plistUrl`，服务端可根据请求 Host 自动给出默认值。
- `bundleIdentifier`、`bundleVersion` 与 plist `title` 不作为长期配置字段，只在生成 plist 时由内部管理页提交，并写入生成后的 plist。

## plist 生成规则

生成标准 iOS OTA manifest plist，核心字段包括：

- `items[0].assets[0].kind = software-package`
- `items[0].assets[0].url = <ipaUrl>`
- `items[0].metadata.bundle-identifier = <bundleIdentifier>`
- `items[0].metadata.bundle-version = <bundleVersion>`
- `items[0].metadata.kind = software`
- `items[0].metadata.title = <title>`

生成前校验：

- 应用名称不能为空。
- bundle identifier 不能为空。
- bundle version 不能为空。
- plist title 不能为空。
- IPA 下载地址不能为空。

生成接口请求示例：

```json
{
  "bundleIdentifier": "com.example.app",
  "bundleVersion": "1.0.0",
  "title": "My iOS App"
}
```

说明：

- `bundleIdentifier`、`bundleVersion` 和 `title` 仅在生成 plist 时必填。
- 后续如果只修改展示名称、IPA 下载地址或 plist 访问地址，不重新生成 plist 时无需再次提供。
- 如果重新生成 plist，需要再次提供准确的 `bundleIdentifier`、`bundleVersion` 和 `title`。

## 运行配置

二进制同目录放置 `config.json`。如果启动时文件不存在，程序会自动生成默认配置。

示例：

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

说明：

- `ip` 和 `port` 组合为监听地址，例如 `0.0.0.0:8080`。
- `dataDir` 如果是相对路径，会按 `config.json` 所在目录解析。
- `auth.user` 和 `auth.password` 用于 Basic Auth。
- `maxUploadBytes` 默认为 `2147483648`，即 `2GB`。

仍保留环境变量覆盖能力，优先级高于 `config.json`：

- `IOSPUB_CONFIG_PATH`
- `IOSPUB_IP`
- `IOSPUB_PORT`
- `IOSPUB_ADDR`
- `IOSPUB_ADMIN_USER`
- `IOSPUB_ADMIN_PASSWORD`
- `IOSPUB_DATA_DIR`
- `IOSPUB_MAX_UPLOAD_BYTES`

## Basic Auth

通过同目录 `config.json` 配置账号密码：

- `auth.user`
- `auth.password`

如果未配置：

- 开发环境可使用默认值 `admin/admin`。
- 启动日志需要明确提示当前使用默认密码。

## HTTPS 与部署

- 应用自身不提供 HTTPS。
- 部署时由 Nginx、Caddy 或其他反向代理负责 HTTPS 终止。
- `ipaUrl` 和 `plistUrl` 应填写代理后的公网 HTTPS 地址，确保 iOS OTA 安装可用。

## 目录结构

建议目录如下：

```text
.
├── cmd/
│   └── iospublisher/
│       └── main.go
├── internal/
│   ├── auth/
│   ├── config/
│   ├── plist/
│   ├── server/
│   └── storage/
├── web/
│   ├── publish.html
│   ├── internal.html
│   ├── app.js
│   └── style.css
├── data/
│   ├── config.json
│   ├── app.ipa
│   └── manifest.plist
└── doc/
    └── ios-publisher-plan.md
```

其中 `data` 为运行时目录，不建议提交上传后的 IPA。

## 路由设计

| 方法 | 路由 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/publish` | 否 | 公开发布页 |
| `GET` | `/internal` | 是 | 内部管理页 |
| `GET` | `/api/state` | 是 | 当前配置和文件状态 |
| `POST` | `/api/upload` | 是 | 上传 IPA |
| `POST` | `/api/config` | 是 | 保存配置 |
| `POST` | `/api/plist/generate` | 是 | 提交 bundle 信息并生成 plist |
| `GET` | `/install` | 否 | 短安装链接，跳转到 iOS OTA 安装地址 |
| `GET` | `/files/app.ipa` | 否 | 当前 IPA 下载 |
| `GET` | `/manifest.plist` | 否 | 当前 plist 下载 |
| `GET` | `/qr.png` | 否 | 当前安装二维码 |

## 实现步骤

1. 初始化 Go 模块和基础目录。
2. 实现配置读写，保存到 `data/config.json`。
3. 实现 Basic Auth 中间件。
4. 实现 IPA 上传接口，限制最大 `2GB`，覆盖保存为单个 `data/app.ipa`。
5. 实现 plist 生成逻辑和校验。
6. 实现公开文件服务：`/files/app.ipa` 和 `/manifest.plist`。
7. 实现安装二维码生成接口。
8. 实现嵌入式前端页面。
9. 实现 `/publish` 和 `/internal` 页面路由。
10. 增加基础测试，覆盖配置读写、鉴权和 plist 生成。
11. 验证 `go build` 可以输出单个二进制。

## 验收标准

- `go build` 后生成一个可执行二进制。
- 启动二进制后可访问 `/publish`。
- `/internal` 未提供 Basic Auth 时返回 `401`。
- 登录 `/internal` 后可以上传 IPA。
- 上传 IPA 默认最大限制为 `2GB`。
- 可以修改应用名称、发布说明、IPA 下载地址和 plist 地址。
- 可以在生成 plist 时填写 `bundleIdentifier`、`bundleVersion` 和 plist `title`。
- 可以生成 `manifest.plist`。
- `/publish` 页面能展示当前应用名称、发布说明、安装短链接和二维码。
- iOS 安装链接格式正确：
  - `/install` 跳转到 `itms-services://?action=download-manifest&url=<plist_url>`
- 应用只保留一个当前 IPA 和一个当前 plist。

## 已确认决策

- `bundleIdentifier`、`bundleVersion` 和 plist `title` 仅在生成 plist 时提供，不作为常规配置长期维护。
- 公开发布页需要展示二维码。
- 公开发布页需要展示发布说明。
- 二维码内容使用内部短链接 `/install`。
- IPA 上传默认限制为 `2GB`。
- 应用无需内置 HTTPS，由部署侧反向代理负责。
