# iOS Publisher

iOS Publisher 是一个轻量级 iOS OTA 发布程序。它使用 Go 标准库实现 HTTP 服务，将前端页面嵌入到单个二进制文件中，适合在内网、测试环境或小型设备上发布 IPA。

项目目标是按标签维护多个当前 IPA 包：`default` 标签保持旧版单包路由兼容，非默认标签用于发布测试包、灰度包或其他并行包。本项目不做历史版本管理、不做多应用管理、不做签名或重签名，也不引入用户权限系统。

多标签发布与 IPA 分析已实现，交付状态见 [doc/delivery-tracking.md](./doc/delivery-tracking.md)。

## 功能

- 公开发布页：`/publish`，展示应用名称、发布时间、发布说明、安装按钮和二维码。
- 多标签发布：`default` 保持旧链接，新增标签使用固定 `fileKey` 命名 IPA 和 plist。
- 管理页：`/internal`，使用 Basic Auth 保护，可按标签维护配置、上传 IPA 和生成 plist。
- IPA 上传：每个标签保留一个当前 IPA，上传完成后记录发布时间。
- IPA 分析：上传后解析 provisioning profile，展示包类型、设备 UUID 列表和证书过期时间。
- UUID 查询：开发包且存在设备 UUID 列表时，发布页可查询 UUID 是否存在。
- plist 生成：根据标签 IPA 地址、Bundle Identifier、Bundle Version 和标题生成 iOS OTA manifest。
- 短安装链接：`/install` 指向 `default`，`/install?tag=<tag>` 指向指定标签。
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
2. 在状态卡片下方选择标签；首次使用只有 `default` 标签。
3. 如需并行发布其他 IPA，新增英文短标识标签，例如 `beta` 或 `qa`。
4. 在当前标签配置区域填写展示名称、发布说明、IPA 地址和 plist 地址。
5. 上传 `.ipa` 文件，上传完成后系统记录发布时间并分析 IPA。
6. 填写 `Bundle Identifier`、`Bundle Version` 和 `plist Title`，点击生成 plist。
7. 打开 `/publish`，用 iPhone 点击安装或扫描对应标签二维码。

如果 IPA 和 plist 都由本服务提供，`default` 标签通常可以使用：

- IPA 地址：`https://your-domain.example/files/app.ipa`
- plist 地址：`https://your-domain.example/manifest.plist`

非默认标签会使用固定 `fileKey`：

- IPA 地址：`https://your-domain.example/files/app-a1b2c3d4.ipa`
- plist 地址：`https://your-domain.example/manifest-a1b2c3d4.plist`
- 安装短链：`https://your-domain.example/install?tag=beta`

如果 IPA 存放在 NAS、对象存储或其他远端地址，可以直接在 `IPA 地址` 中填写该 HTTPS 链接。生成 plist 时，如果当前标签没有本地 IPA 文件，服务端会对显式填写的 `ipaUrl` 发起 `HEAD` 检测；返回 `2xx` 或 `3xx` 时允许生成 plist，否则返回错误。`ipaUrl` 留空时表示使用本服务托管文件，仍需要先上传 IPA。

iOS OTA 安装要求可被设备访问的 HTTPS 地址。生产或外网环境建议使用 Nginx、Caddy 等反向代理提供 HTTPS。

## 标签规则

- `default` 标签始终存在且不可删除。
- 标签名只允许字母、数字、横线和下划线。
- 标签名唯一，且不可与 `default` 冲突。
- 新增非默认标签时，服务端生成一个固定 8 位小写字母数字 `fileKey`。
- 删除非默认标签时，同步删除对应 IPA、plist 和分析数据。
- 每个标签只保留一个当前 IPA 和一个当前 plist。

## IPA 分析

上传 IPA 后，服务端会尽力解析 `Payload/*.app/embedded.mobileprovision`：

| 字段 | 说明 |
| --- | --- |
| 包类型 | `development`、`ad-hoc`、`enterprise`、`app-store` 或 `unknown`。 |
| 设备 UUID | 从 `ProvisionedDevices` 提取，开发包用于发布页查询。 |
| 证书过期时间 | 从 `DeveloperCertificates` 解析证书，取最早 `NotAfter`。 |
| 描述文件过期时间 | 从 profile 的 `ExpirationDate` 读取。 |
| 分析状态 | `pending`、`success` 或 `failed`，失败时记录错误原因。 |

分析失败不会阻断上传；管理页会展示失败原因，包类型记为 `unknown`。

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
├── config.json
├── app.ipa
├── manifest.plist
├── app-a1b2c3d4.ipa
└── manifest-a1b2c3d4.plist
```

注意这里的 `data/config.json` 是发布配置，包含标签、展示名称、发布说明、IPA URL、plist URL、发布时间和分析结果；根目录或可执行文件同目录的 `config.json` 是运行配置，包含监听地址、认证和上传限制。

## API 和路由

| 方法 | 路由 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/publish` | 否 | 公开发布页 |
| `GET` | `/internal` | 是 | 管理页 |
| `GET` | `/api/tags` | 是 | 标签列表和摘要状态 |
| `POST` | `/api/tags` | 是 | 新增标签 |
| `DELETE` | `/api/tags?tag=beta` | 是 | 删除非默认标签 |
| `GET` | `/api/state?tag=beta` | 是 | 指定标签配置、文件状态和分析结果 |
| `POST` | `/api/config?tag=beta` | 是 | 保存指定标签发布配置 |
| `POST` | `/api/upload?tag=beta` | 是 | 上传指定标签 IPA |
| `POST` | `/api/plist/generate?tag=beta` | 是 | 生成指定标签 plist |
| `GET` | `/api/publish` | 否 | 公开发布状态，包含所有标签摘要 |
| `GET` | `/api/uuid/search?tag=beta&q=<query>` | 否 | 查询开发包 UUID |
| `GET` | `/files/app.ipa` | 否 | `default` IPA 下载 |
| `GET` | `/files/app-<fileKey>.ipa` | 否 | 非默认标签 IPA 下载 |
| `GET` | `/manifest.plist` | 否 | `default` plist |
| `GET` | `/manifest-<fileKey>.plist` | 否 | 非默认标签 plist |
| `GET` | `/install` | 否 | `default` 安装短链 |
| `GET` | `/install?tag=beta` | 否 | 指定标签安装短链 |
| `GET` | `/qr.png` | 否 | `default` 安装二维码 |
| `GET` | `/qr.png?tag=beta` | 否 | 指定标签安装二维码 |

没有 `tag` 参数时，管理 API 默认操作 `default` 标签。

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

当前测试已覆盖多标签发布与 IPA 分析的核心链路：

- 旧 `default` 发布流程兼容。
- 标签创建、删除和标签间配置隔离。
- 非默认文件命名和公开链接。
- 远端 IPA URL 通过 `HEAD` 检测后生成 plist。
- 发布时间随上传更新。
- 多标签发布页折叠展示。
- IPA 包类型识别、UUID 查询和证书过期时间解析。

## 安全建议

- 上线前一定修改默认账号密码，不要使用 `admin/admin`。
- 外网或 iOS 真机安装场景请使用 HTTPS。
- 不要提交真实 `config.json`、上传后的 IPA、生成的 plist、日志或部署产物。
- 管理页和管理 API 只适合给受信任人员使用。
