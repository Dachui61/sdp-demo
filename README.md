# gRPC 隧道代理系统

本项目实现了一个基于 gRPC 的隧道代理系统，允许通过 SOCKS5 代理将流量通过安全隧道传输。

## 功能特点

- 基于 gRPC 的安全隧道传输
- SOCKS5 代理支持
- 本地流量转发
- 轻量级架构设计

## 系统组件

### 客户端 (client.exe)
在本地启动 SOCKS5 代理服务器，监听端口 1081。

### 网关 (gateway.exe)
接收来自客户端的连接请求，并通过 gRPC 隧道转发流量。

## 快速开始

### 1. 构建项目

```bash
# 构建网关
go build -o gateway.exe ./cmd/gateway

# 构建客户端
go build -o client.exe ./cmd/client
```

### 2. 启动系统

```bash
# 启动网关 (终端 1)
./gateway.exe

# 启动客户端 (终端 2)
./client.exe
```

### 3. 配置代理

在需要使用代理的应用程序中配置 SOCKS5 代理：
- 地址: 127.0.0.1
- 端口: 1081

## 使用说明

详细使用说明请参考以下文档：
- [系统使用说明](SYSTEM_USAGE.md)
- [SOCKS5 代理使用说明](SOCKS5_PROXY_USAGE.md)
- [浏览器配置指南](BROWSER_CONFIG.md)

## 测试工具

项目包含多个测试工具帮助验证系统功能：
- `proxy_demo.py` - Python SOCKS5 代理演示脚本
- `test_socks5_proxy.py` - 自动化测试脚本
- `test-proxy.ps1` - PowerShell 代理测试脚本

## 日志监控

### 客户端日志
```
2025/09/12 22:01:30 Starting client. SOCKS5 at 127.0.0.1:1081
2025/09/12 22:01:30 Starting SOCKS5 server on 127.0.0.1:1081
```

### 网关日志
```
2025/09/12 22:00:25 [gateway] gRPC listening :50051
2025/09/12 22:00:25 [gateway] starting dummy backend on :9000
2025/09/12 22:00:45 [gateway] new gRPC tunnel stream established
```

## 故障排除

如果遇到问题，请检查：
1. 客户端和网关程序是否都在运行
2. 防火墙设置是否阻止了端口访问
3. 端口是否被其他程序占用

查看运行中的进程：
```bash
tasklist | findstr -i "client gateway"
```