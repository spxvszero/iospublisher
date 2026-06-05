# iOS Publisher

iOS Publisher 是一个轻量级 iOS OTA 发布程序。它使用 Go 标准库实现 HTTP 服务，将前端页面嵌入到单个二进制文件中，适合在内网、测试环境或小型设备上发布一个当前版本的 IPA。

项目只维护一个当前 IPA 和一个当前 `manifest.plist`，不做多应用、多版本、签名、重签名或用户权限系统。

## 功能

- 公开发布页：`/publish`，展示应用名称、发布说明、安装按钮和二维码。
- 管理页：`/internal`，使用 Basic Auth 保护。
- IPA 上传：上传后保存为当前 `app.ipa`。
- plist 生成：根据 IPA 地址、Bundle Identifier、Bundle Version 和标题生成 iOS OTA manifest。
- 短安装链接：`/install` 会跳转到 `itms-services://?action=download-manifest&url=<plist_url>`。
- 运行时配置：通过 `config.json` 或环境变量配置监听地址、账号密码、数据目录和上传大小。

## 快速开始

### 1. 准备配置

复制示例配置并修改管理员账号密码：

```powershell
Copy-Item config.json.example config.json
```

Linux/macOS：

```sh
cp config.json.example config.json
```

`config.json` 会被 Git 忽略，适合放置本机或部署环境的真实配置。如果启动时找不到配置文件，程序也会自动生成一份默认配置。

### 2. 构建

```powershell
go build -o iospublisher.exe ./cmd/iospublisher
```

Linux/macOS：

```sh
go build -o iospublisher ./cmd/iospublisher
```

### 3. 启动

```powershell
.\iospublisher.exe
```

Linux/macOS：

```sh
./iospublisher
```

启动后访问：

- 发布页：`http://localhost:8080/publish`
- 管理页：`http://localhost:8080/internal`

## 发布流程

1. 打开 `/internal`，输入 `config.json` 中配置的 Basic Auth 账号密码。
2. 在配置区域填写展示名称、发布说明、IPA 地址和 plist 地址。
3. 上传 `.ipa` 文件。
4. 填写 `Bundle Identifier`、`Bundle Version` 和 `plist Title`，点击生成 plist。
5. 打开 `/publish`，用 iPhone 点击安装或扫描二维码。

如果 IPA 和 plist 都由本服务提供，通常可以使用：

- IPA 地址：`https://your-domain.example/files/app.ipa`
- plist 地址：`https://your-domain.example/manifest.plist`

iOS OTA 安装要求可被设备访问的 HTTPS 地址。生产或外网环境建议使用 Nginx、Caddy 等反向代理提供 HTTPS。

## 运行配置

运行配置文件默认位于可执行文件同目录的 `config.json`。也可以用 `IOSPUB_CONFIG_PATH` 指定其他路径。

参考 [config.json.example](./config.json.example)：

```json
{
  "ip": "0.0.0.0",
  "port": 8080,
  "dataDir": "data",
  "auth": {
    "user": "admin",
    "password": "please-change-me"
  },
  "maxUploadBytes": 2147483648
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `ip` | 监听 IP，例如 `0.0.0.0` 表示监听所有网卡。 |
| `port` | 监听端口，范围为 `1` 到 `65535`。 |
| `dataDir` | 运行数据目录。相对路径会按 `config.json` 所在目录解析。 |
| `auth.user` | 管理页和管理 API 的 Basic Auth 用户名。 |
| `auth.password` | 管理页和管理 API 的 Basic Auth 密码。 |
| `maxUploadBytes` | 单个 IPA 上传大小限制，默认 `2147483648`，即 2GB。 |

环境变量优先级高于 `config.json`：

| 环境变量 | 说明 |
| --- | --- |
| `IOSPUB_CONFIG_PATH` | 指定运行配置文件路径。 |
| `IOSPUB_IP` | 覆盖监听 IP。 |
| `IOSPUB_PORT` | 覆盖监听端口。 |
| `IOSPUB_ADDR` | 直接覆盖最终监听地址，例如 `127.0.0.1:18080`。 |
| `IOSPUB_DATA_DIR` | 覆盖数据目录。 |
| `IOSPUB_ADMIN_USER` | 覆盖 Basic Auth 用户名。 |
| `IOSPUB_ADMIN_PASSWORD` | 覆盖 Basic Auth 密码。 |
| `IOSPUB_MAX_UPLOAD_BYTES` | 覆盖最大上传字节数。 |

## 数据文件

默认数据目录为 `data`：

```text
data/
├── app.ipa
├── config.json
└── manifest.plist
```

注意这里的 `data/config.json` 是发布页配置，包含展示名称、发布说明、IPA URL、plist URL 和更新时间；根目录或可执行文件同目录的 `config.json` 是运行配置，包含监听地址、认证和上传限制。

## API 和路由

| 方法 | 路由 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/publish` | 否 | 公开发布页 |
| `GET` | `/internal` | 是 | 管理页 |
| `GET` | `/api/state` | 是 | 当前配置和文件状态 |
| `POST` | `/api/config` | 是 | 保存发布页配置 |
| `POST` | `/api/upload` | 是 | 上传 IPA |
| `POST` | `/api/plist/generate` | 是 | 生成 `manifest.plist` |
| `GET` | `/api/publish` | 否 | 公开发布状态 |
| `GET` | `/files/app.ipa` | 否 | 当前 IPA 下载 |
| `GET` | `/manifest.plist` | 否 | 当前 plist |
| `GET` | `/manifest.plist?download=1` | 否 | 下载当前 plist |
| `GET` | `/install` | 否 | 跳转到 iOS OTA 安装链接 |
| `GET` | `/qr.png` | 否 | 当前安装二维码 |

## 交叉编译

R4S 或其他 Linux ARM64 设备：

```powershell
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-s -w" -o dist/iospublisher-r4s-linux-arm64/iospublisher ./cmd/iospublisher
```

更多说明见 [doc/r4s-deploy.md](./doc/r4s-deploy.md)。

## 测试

```powershell
go test ./...
```

## 安全建议

- 上线前一定修改默认账号密码，不要使用 `admin/admin`。
- 外网或 iOS 真机安装场景请使用 HTTPS。
- 不要提交真实 `config.json`、上传后的 IPA、日志或部署产物。
- 管理页和管理 API 只适合给受信任人员使用。
