# R4S 部署说明

## 目标平台

NanoPi R4S 常见系统为 Linux ARM64/aarch64。本项目为纯 Go 实现，前端资源已嵌入二进制，可直接交叉编译为 Linux ARM64 单文件程序。

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

## 32 位系统

如果 R4S 系统是 32 位 ARM，使用以下构建参数：

```powershell
$env:GOOS = "linux"
$env:GOARCH = "arm"
$env:GOARM = "7"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-s -w" -o dist/iospublisher-linux-armv7/iospublisher ./cmd/iospublisher
```
