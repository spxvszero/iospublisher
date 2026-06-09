# R4S 部署说明

## 目标平台

NanoPi R4S 常见系统为 Linux ARM64/aarch64。本项目为纯 Go 实现，前端资源已嵌入二进制，可直接交叉编译为 Linux ARM64 单文件程序。

多标签发布与 IPA 分析功能仍保持单二进制部署方式，不需要数据库、外部服务或额外系统命令。

## 构建命令

```powershell
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-s -w" -o dist/iospublisher-r4s-linux-arm64/iospublisher ./cmd/iospublisher
```

## 运行配置

部署包内包含 `config.json`：

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

建议上线前修改 `auth.user` 和 `auth.password`。

## R4S 运行

```sh
tar -xzf iospublisher-r4s-linux-arm64.tar.gz
cd iospublisher-r4s-linux-arm64
chmod +x iospublisher
./iospublisher
```

访问：

- `http://<R4S_IP>:8080/publish`
- `http://<R4S_IP>:8080/internal`

## 多标签数据目录

默认数据目录仍为运行配置中的 `dataDir`。多标签功能会在同一目录下保存各标签当前包、plist 和发布配置。

示例：

```text
data/
├── config.json
├── app.ipa
├── manifest.plist
├── app-a1b2c3d4.ipa
└── manifest-a1b2c3d4.plist
```

说明：

- `app.ipa` 和 `manifest.plist` 属于 `default` 标签，用于兼容旧公开链接。
- `app-<fileKey>.ipa` 和 `manifest-<fileKey>.plist` 属于非默认标签。
- `fileKey` 为新增标签时固定生成的 8 位小写字母数字串。
- `data/config.json` 保存多标签发布配置、发布时间和 IPA 分析结果。

## HTTPS 与存储建议

- iOS OTA 安装要求设备可访问的 HTTPS 地址，建议继续由 Nginx、Caddy 或其他反向代理提供 HTTPS。
- 多标签会增加磁盘占用，每个标签保留一个当前 IPA 和一个当前 plist。
- R4S 部署时建议将 `dataDir` 放在容量足够且可靠的存储介质上。
- 上传的 IPA、生成的 plist 和 `data/config.json` 都不建议提交到 Git。

## 32 位系统

如果 R4S 系统是 32 位 ARM，使用以下构建参数：

```powershell
$env:GOOS = "linux"
$env:GOARCH = "arm"
$env:GOARM = "7"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-s -w" -o dist/iospublisher-linux-armv7/iospublisher ./cmd/iospublisher
```
