# iOS 发布程序交付追踪

## 交付阶段

| 阶段 | 状态 | 记录 |
| --- | --- | --- |
| 1. 初始化 Go 项目结构 | 已完成 | 已创建 Go 模块、后端目录、前端目录和运行时目录约定。 |
| 2. 后端核心能力 | 已完成 | 已实现配置读写、Basic Auth、IPA 上传、plist 生成、二维码生成。 |
| 3. 嵌入式前端页面 | 已完成 | 已实现 `/publish` 公开页与 `/internal` 管理页，前端资源通过 Go `embed` 内嵌。 |
| 4. 测试与构建 | 已完成 | `go test ./...` 通过，`go build ./cmd/iospublisher` 通过。 |
| 5. 交付验收记录 | 已完成 | 已记录 v1 功能、测试、构建结果和本地验证结果。 |
| 6. 多标签发布与 IPA 分析 | 已完成 | 已实现标签、发布时间、IPA 分析、UUID 查询和前端展示。 |

## v1 验收项

| 验收项 | 状态 | 记录 |
| --- | --- | --- |
| 单个 Go 二进制可构建 | 已通过 | `go build ./cmd/iospublisher` 通过。 |
| `/publish` 可公开访问 | 已通过 | 路由保留公开访问。 |
| `/internal` 受 Basic Auth 保护 | 已通过 | 单元测试覆盖未认证和认证访问。 |
| 可上传默认 IPA，默认限制 `2GB` | 已通过 | 上传接口限制默认值为 `2147483648` 字节。 |
| 可修改展示名称、IPA 地址和 plist 地址 | 已通过 | `/api/config` 测试和前端管理页均已覆盖。 |
| 生成 plist 时填写 `bundleIdentifier` 和 `bundleVersion` | 已通过 | `/api/plist/generate` 测试覆盖生成逻辑。 |
| `/manifest.plist` 可公开访问 | 已通过 | 端到端测试覆盖生成后公开访问。 |
| `/qr.png` 可生成安装二维码 | 已通过 | 二维码 PNG 生成测试和端到端测试均已覆盖。 |
| 前端资源嵌入 Go 二进制 | 已通过 | `web` 包使用 `//go:embed` 嵌入 HTML、CSS、JS。 |
| `/publish` 不提供 plist 直接下载入口 | 已通过 | 公开页仅保留安装按钮、安装链接和二维码。 |
| 管理页生成 plist 后提供下载按钮 | 已通过 | `/internal` 的 plist 区域在检测到已生成 plist 后显示下载按钮。 |
| 二维码使用内部短链 | 已通过 | `/install` 由服务端跳转到实际 `itms-services` 地址。 |
| 管理页可维护发布说明 | 已通过 | 配置中包含 `releaseNotes`，发布页展示发布说明。 |
| 生成 plist 时可配置 title | 已通过 | `/api/plist/generate` 支持 `title` 参数。 |
| 二进制同目录运行配置 | 已通过 | 同目录 `config.json` 可配置 IP、端口、Basic Auth、数据目录和上传限制。 |
| HTTPS 由部署代理负责 | 已确认 | 应用自身不内置 HTTPS。 |

## v2 验收项

| 验收项 | 状态 | 记录 |
| --- | --- | --- |
| `default` 标签兼容旧流程 | 已通过 | `/install`、`/qr.png`、`/files/app.ipa`、`/manifest.plist` 继续指向 default。 |
| 可新增非默认标签 | 已通过 | 标签名校验为英文短标识，新增时固定绑定 8 位随机 `fileKey`。 |
| 可删除非默认标签 | 已通过 | 删除时同步清理对应 IPA、plist 和分析数据，`default` 不可删除。 |
| 标签间配置隔离 | 已通过 | 状态、配置、上传、plist、链接和分析结果均按当前标签读取。 |
| 非默认文件命名 | 已通过 | 文件名为 `app-<fileKey>.ipa` 和 `manifest-<fileKey>.plist`。 |
| 远端 IPA 生成 plist | 已通过 | 未上传本地 IPA 但显式填写 `ipaUrl` 时，生成 plist 前通过 `HEAD` 检测远端链接。 |
| 发布时间展示 | 已通过 | 上传 IPA 后写入 `publishedAt`，发布页和折叠标题显示该时间。 |
| 多标签发布页折叠 | 已通过 | 只有 default 时保持单卡片；多标签时 default 展开，其他标签折叠。 |
| IPA 包类型分析 | 已通过 | 识别 development、ad-hoc、enterprise、app-store 和 unknown。 |
| UUID 查询 | 已通过 | 开发包显示查询，最少 4 字符，大小写不敏感，最多 20 条结果。 |
| 证书过期时间 | 已通过 | 解析 `DeveloperCertificates` 最早 `NotAfter`，并记录 profile `ExpirationDate`。 |
| 分析失败处理 | 已通过 | 分析失败不阻断上传，管理页展示失败原因。 |

## 验证记录

### 2026-06-08 多标签与 IPA 分析实现

- `go test ./...`：通过。
- `go build ./cmd/iospublisher`：通过。
- `node --check web/app.js`：通过。
- 已验证远端 IPA URL 可通过 `HEAD` 检测后生成 plist，且 `ipaUrl` 留空时仍要求先上传 IPA。
- 新增 `internal/ipa`，支持 IPA 解包、provisioning profile 提取、包类型判断、证书过期时间解析和 UUID 列表提取。
- `internal/config` 已升级为 `schemaVersion: 2` 多标签配置，并兼容旧版扁平 `data/config.json` 自动迁移到 `default`。
- `internal/server` 已新增标签 CRUD、按标签上传、按标签生成 plist、按标签安装/二维码、公开 UUID 查询和多标签发布状态。
- `web` 已新增内部管理标签页、分析结果展示、发布时间展示、公开发布页折叠卡片和 UUID 查询。

### 2026-06-08 多标签与 IPA 分析文档

- 已更新产品计划，新增标签模型、公开链接兼容策略、发布时间、IPA 分析和 UUID 查询规则。
- 已更新 README 和 R4S 部署说明，避免说明停留在默认单包口径。

### 2026-05-27 v1 验证

- `go test ./...`：通过。
- `go build ./cmd/iospublisher`：通过。
- 本地服务冒烟：
  - `GET http://127.0.0.1:18080/publish` 返回 `200`。
  - `GET http://127.0.0.1:18080/internal` 未认证返回 `401`。
  - `GET http://127.0.0.1:18080/internal` 使用 `admin/admin` 返回 `200`。
  - `GET http://127.0.0.1:18080/api/publish` 返回默认发布状态，未上传 IPA 时 `ready=false`。

## 已实现文件

- `cmd/iospublisher/main.go`：程序入口、运行配置读取、环境变量覆盖、HTTP 服务启动。
- `internal/auth`：Basic Auth 中间件。
- `internal/config`：多标签 JSON 配置读写、旧配置迁移、标签创建删除和 `fileKey` 生成。
- `internal/ipa`：IPA 解包、provisioning profile 提取、包类型判断、证书过期时间解析和 UUID 列表提取。
- `internal/runtimeconfig`：二进制同目录运行配置读写。
- `internal/plist`：iOS OTA manifest plist 生成。
- `internal/qrcode`：离线二维码 PNG 生成。
- `internal/server`：路由、上传、文件访问、标签 API、发布 API 和 UUID 查询 API。
- `web`：嵌入式前端页面与静态资源。

## 运行约定

- 默认运行配置文件：二进制同目录 `config.json`。
- 默认监听 IP：`0.0.0.0`。
- 默认监听端口：`8080`。
- 默认管理账号：`admin`。
- 默认管理密码：`admin`。
- 默认数据目录：`data`，相对路径按 `config.json` 所在目录解析。
- 默认上传限制：`2147483648` 字节，即 `2GB`。
- `default` 标签默认文件：`data/app.ipa` 和 `data/manifest.plist`。
- 非默认标签默认文件：`data/app-<fileKey>.ipa` 和 `data/manifest-<fileKey>.plist`。
