# 轻量 iOS 发布程序计划

## 目标

实现一个 Go 编写的轻量 iOS OTA 发布程序，最终交付为单个可执行二进制文件。程序内嵌前端页面，支持按标签维护多个当前 IPA 包，`default` 标签保持旧版单包行为和旧公开链接兼容。

本项目只做“每个标签一个当前包”，不做历史版本管理、不做多应用管理、不做签名或重签名。

## 功能范围

### 公开发布页

- 路由：`GET /publish`
- 无需登录。
- 展示每个可发布标签的应用名称、安装入口、安装二维码、发布时间和发布说明。
- 发布时间以对应标签 IPA 上传完成时间为准。
- 如果只有 `default` 标签，页面保持当前单卡片展示方式，仅新增发布时间。
- 如果存在多个标签，`default` 标签默认展开展示；其他标签折叠显示，折叠标题为 `应用名称 + 标签名 + 发布时间`。
- 点击展开非默认标签后，显示该标签自己的安装按钮、二维码、发布说明、发布时间和 UUID 查询能力。
- 如果某个标签尚未上传 IPA 或尚未生成 plist，该标签展示未配置状态，不提供安装入口。
- 开发包且解析到设备 UUID 列表时，发布页显示 UUID 查询框；企业包、App Store 包、未知类型或无 UUID 列表时隐藏查询框。

### 内部管理页

- 路由：`GET /internal`
- 使用 Basic Auth 保护。
- 在状态卡片下方、配置卡片上方新增标签选择页。
- 默认存在 `default` 标签，且 `default` 标签不可删除。
- 可新增和删除非默认标签；标签名使用英文短标识，只允许字母、数字、横线和下划线。
- 新增标签时，服务端为该标签生成并固定绑定一个 8 位随机小写字母数字 `fileKey`。
- 切换标签后，状态卡片、配置表单、IPA 上传、plist 生成、链接区域和 IPA 分析结果都切换到该标签。
- 上传 IPA 后，后端保存文件、更新时间并尽力分析 IPA 内容；分析失败不阻断上传，但需要在管理页展示错误状态。

### 标签模型

- `default` 标签复用旧版文件和公开链接：
  - IPA：`data/app.ipa`
  - plist：`data/manifest.plist`
  - 安装短链：`/install`
  - 二维码：`/qr.png`
- 非默认标签使用固定 `fileKey` 生成文件名：
  - IPA：`data/app-<fileKey>.ipa`
  - plist：`data/manifest-<fileKey>.plist`
  - IPA 下载：`/files/app-<fileKey>.ipa`
  - plist 访问：`/manifest-<fileKey>.plist`
  - 安装短链：`/install?tag=<tag>`
  - 二维码：`/qr.png?tag=<tag>`
- 删除非默认标签时，同步删除该标签对应的 IPA、plist 和分析数据。

### IPA 分析模块

上传 IPA 后，服务端打开 IPA 压缩包并查找 `Payload/*.app/embedded.mobileprovision`。分析模块从 provisioning profile 中提取 plist 内容，解析以下信息：

- 包类型：
  - `development`：存在 `ProvisionedDevices`，且 `Entitlements.get-task-allow = true`。
  - `ad-hoc`：存在 `ProvisionedDevices`，且 `Entitlements.get-task-allow` 不是 `true`。
  - `enterprise`：`ProvisionsAllDevices = true`。
  - `app-store`：不存在 `ProvisionedDevices`，且不是企业包。
  - `unknown`：未找到 profile、解析失败或字段不足。
- 设备 UUID 列表：读取 `ProvisionedDevices`，仅开发包在发布页提供查询。
- 证书过期时间：读取 `DeveloperCertificates` 并用 `crypto/x509` 解析证书，取最早的 `NotAfter`。
- 描述文件过期时间：记录 `ExpirationDate`，作为证书过期时间之外的辅助信息。
- 分析状态：记录 `pending`、`success` 或 `failed`，失败时保存错误原因。

公开页 UUID 查询规则：

- 查询接口按标签查询。
- 查询词至少 4 个字符。
- 大小写不敏感，支持模糊匹配。
- 最多返回 20 条匹配 UUID。
- 只在开发包且存在 UUID 列表时显示查询入口。

### 管理接口

所有管理接口均走 Basic Auth。没有 `tag` 参数时默认操作 `default` 标签。

| 方法 | 路由 | 说明 |
| --- | --- | --- |
| `GET` | `/api/tags` | 读取标签列表和摘要状态 |
| `POST` | `/api/tags` | 新增标签，请求体为 `{ "name": "beta" }` |
| `DELETE` | `/api/tags?tag=beta` | 删除非默认标签 |
| `GET` | `/api/state?tag=beta` | 读取指定标签配置、文件状态和分析结果 |
| `POST` | `/api/config?tag=beta` | 保存指定标签的展示配置 |
| `POST` | `/api/upload?tag=beta` | 上传指定标签的 IPA |
| `POST` | `/api/plist/generate?tag=beta` | 为指定标签生成 plist |

### 公开接口和文件访问

| 方法 | 路由 | 说明 |
| --- | --- | --- |
| `GET` | `/publish` | 公开发布页 |
| `GET` | `/api/publish` | 公开发布状态，返回所有标签摘要和 default 兼容字段 |
| `GET` | `/api/uuid/search?tag=beta&q=<query>` | 查询开发包 UUID 是否存在 |
| `GET` | `/install` | `default` 标签安装短链 |
| `GET` | `/install?tag=beta` | 指定标签安装短链 |
| `GET` | `/qr.png` | `default` 标签安装二维码 |
| `GET` | `/qr.png?tag=beta` | 指定标签安装二维码 |
| `GET` | `/files/app.ipa` | `default` 标签 IPA 下载 |
| `GET` | `/files/app-<fileKey>.ipa` | 非默认标签 IPA 下载 |
| `GET` | `/manifest.plist` | `default` 标签 plist |
| `GET` | `/manifest-<fileKey>.plist` | 非默认标签 plist |

`/api/publish` 需要保留旧版单包字段，便于 `default` 兼容；同时新增 `tags` 数组，供多标签发布页渲染。

## 配置模型

`data/config.json` 迁移为多标签结构。读取旧版扁平配置时，服务端自动迁移为 `default` 标签。

建议结构：

```json
{
  "schemaVersion": 2,
  "activeTag": "default",
  "tags": [
    {
      "name": "default",
      "fileKey": "",
      "config": {
        "appName": "My iOS App",
        "releaseNotes": "本次发布说明",
        "ipaUrl": "https://example.com/files/app.ipa",
        "plistUrl": "https://example.com/manifest.plist",
        "updatedAt": "2026-05-27T00:00:00Z",
        "publishedAt": "2026-06-08T08:30:00Z"
      },
      "analysis": {
        "status": "success",
        "packageType": "development",
        "deviceUUIDs": ["00000000-0000000000000000000000000000000000000000"],
        "certificateExpiresAt": "2027-06-08T00:00:00Z",
        "profileExpiresAt": "2027-06-08T00:00:00Z",
        "analyzedAt": "2026-06-08T08:30:02Z",
        "error": ""
      }
    },
    {
      "name": "beta",
      "fileKey": "a1b2c3d4",
      "config": {
        "appName": "My iOS App",
        "releaseNotes": "",
        "ipaUrl": "https://example.com/files/app-a1b2c3d4.ipa",
        "plistUrl": "https://example.com/manifest-a1b2c3d4.plist",
        "updatedAt": "2026-06-08T08:00:00Z",
        "publishedAt": ""
      },
      "analysis": {
        "status": "pending",
        "packageType": "unknown",
        "deviceUUIDs": [],
        "certificateExpiresAt": "",
        "profileExpiresAt": "",
        "analyzedAt": "",
        "error": ""
      }
    }
  ]
}
```

说明：

- `updatedAt` 表示配置保存时间。
- `publishedAt` 表示 IPA 上传完成时间，并用于公开发布页展示。
- `fileKey` 只在新增标签时生成一次；`default` 标签为空。
- 用户未手动填写 `ipaUrl` 或 `plistUrl` 时，服务端根据标签和请求 Host 自动给出默认值。
- `bundleIdentifier`、`bundleVersion` 与 plist `title` 仍只在生成 plist 时提交，不作为长期配置字段。

## plist 生成规则

每个标签独立生成标准 iOS OTA manifest plist，核心字段包括：

- `items[0].assets[0].kind = software-package`
- `items[0].assets[0].url = <tag.ipaUrl>`
- `items[0].metadata.bundle-identifier = <bundleIdentifier>`
- `items[0].metadata.bundle-version = <bundleVersion>`
- `items[0].metadata.kind = software`
- `items[0].metadata.title = <title>`

生成前校验：

- 标签必须存在。
- 应用名称不能为空。
- bundle identifier 不能为空。
- bundle version 不能为空。
- plist title 不能为空。
- IPA 下载地址不能为空。
- 当前标签已经上传 IPA，或显式填写的远端 `ipaUrl` 可以通过 `HEAD` 访问。
- `ipaUrl` 留空时表示使用本服务托管文件，仍必须先上传 IPA。

生成接口请求示例：

```json
{
  "bundleIdentifier": "com.example.app",
  "bundleVersion": "1.0.0",
  "title": "My iOS App"
}
```

## 技术方案

### 后端

- 语言：Go。
- HTTP 框架：优先使用标准库 `net/http`，减少依赖，保持二进制轻量。
- 前端嵌入：使用 `embed` 包，通过 `//go:embed` 将静态资源嵌入二进制。
- 运行配置：二进制同目录使用 `config.json`，用于配置监听 IP、端口、Basic Auth 账号密码、数据目录和最大上传大小。
- 配置持久化：使用本地 JSON 文件 `data/config.json`，多标签配置和分析结果都保存在该文件。
- 上传文件存储：按标签保存到 `data/app.ipa` 或 `data/app-<fileKey>.ipa`。
- plist 输出：按标签保存到 `data/manifest.plist` 或 `data/manifest-<fileKey>.plist`。
- IPA 分析：新增 `internal/ipa` 模块，使用 `archive/zip` 读取 IPA，使用 plist/XML 解析和 `crypto/x509` 解析证书。
- 二维码输出：由服务端根据对应标签短安装链接生成 PNG，避免公开页依赖外部 CDN。

### 前端

- 使用原生 HTML/CSS/JavaScript 或极轻量构建方式。
- 页面作为静态资源嵌入 Go。
- `/internal` 通过标签页驱动当前管理上下文。
- `/publish` 根据 `/api/publish` 返回的标签数量决定单卡片或多折叠展示。
- 前端通过 fetch 调用管理 API 和公开 UUID 查询 API。

### 单二进制交付

- 前端源码在构建时嵌入 Go。
- 运行时只依赖一个二进制文件。
- 多标签和 IPA 分析功能不引入数据库，不要求额外系统命令或外部服务。

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

仍保留环境变量覆盖能力，优先级高于 `config.json`：

- `IOSPUB_CONFIG_PATH`
- `IOSPUB_IP`
- `IOSPUB_PORT`
- `IOSPUB_ADDR`
- `IOSPUB_ADMIN_USER`
- `IOSPUB_ADMIN_PASSWORD`
- `IOSPUB_DATA_DIR`
- `IOSPUB_MAX_UPLOAD_BYTES`

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
│   ├── ipa/
│   ├── plist/
│   ├── qrcode/
│   ├── runtimeconfig/
│   └── server/
├── web/
│   ├── publish.html
│   ├── internal.html
│   ├── app.js
│   └── style.css
├── data/
│   ├── config.json
│   ├── app.ipa
│   ├── app-a1b2c3d4.ipa
│   ├── manifest.plist
│   └── manifest-a1b2c3d4.plist
└── doc/
    └── ios-publisher-plan.md
```

其中 `data` 为运行时目录，不建议提交上传后的 IPA、plist 或真实配置。

## 实现步骤

1. 将配置存储从扁平 `Config` 升级为多标签 `Store`，并兼容迁移旧 `data/config.json`。
2. 实现标签校验、创建、删除和 `fileKey` 生成。
3. 将上传、plist 生成、文件访问、安装短链和二维码逻辑按标签隔离。
4. 上传成功后写入 `publishedAt`，并触发 IPA 分析。
5. 新增 IPA 分析模块和 UUID 搜索接口。
6. 更新 `/internal` 标签页、状态卡片、配置表单、上传、plist、链接和分析结果展示。
7. 更新 `/publish` 单标签兼容展示和多标签折叠展示。
8. 更新 README、部署文档和交付追踪。
9. 增加后端单元测试、端到端测试和前端冒烟验证。
10. 验证 `go test ./...` 和 `go build ./cmd/iospublisher`。

## 验收标准

- 旧版 `default` 流程保持可用：`/install`、`/qr.png`、`/files/app.ipa`、`/manifest.plist` 不变。
- 可以创建非默认标签，并生成固定 8 位 `fileKey`。
- 非默认标签默认 IPA 文件名为 `app-<fileKey>.ipa`。
- 切换标签后，配置、状态、上传、plist、链接和分析结果互不串用。
- 删除非默认标签会清理对应文件和配置；`default` 标签不可删除。
- `/publish` 在单标签时保持旧布局并显示发布时间。
- `/publish` 在多标签时默认展开 `default`，其他标签折叠标题显示应用名称、标签名和发布时间。
- 上传 IPA 后，发布时间随该标签更新。
- 上传 IPA 后，管理页显示包类型、证书过期时间、描述文件过期时间和分析状态。
- 开发包在发布页显示 UUID 查询，查询至少 4 个字符，最多返回 20 条匹配结果。
- 企业包、App Store 包、未知类型或无 UUID 列表时，发布页不显示 UUID 查询。
- IPA 分析失败不阻断上传，管理页能展示失败原因。

## 已确认决策

- 标签名采用英文短标识：字母、数字、横线、下划线，唯一且不可与 `default` 冲突。
- `default` 标签保持旧路由和旧文件名兼容。
- 8 位随机字符串使用 `crypto/rand` 生成的小写字母数字串，生成后固定绑定标签。
- 发布时间以 IPA 上传完成时间为准。
- UUID 查询只在解析到开发包且存在 UUID 列表时显示。
- 本阶段不引入历史版本、用户系统、复杂权限、数据库、签名或重签名能力。
- 应用无需内置 HTTPS，由部署侧反向代理负责。
