# 1. 选择官方 Go 镜像 (包含 Go 1.22+ 和 Go Modules)
FROM golang:1.22-bullseye

# 2. 设置环境变量 (Go Modules 默认启用)
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=1

# 3. 安装 Git
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git ca-certificates openssh-client && \
    rm -rf /var/lib/apt/lists/*

# 4. 设置工作目录
WORKDIR /app

# 5. 把代码挂载到 /app 时，默认 go mod 会在这里工作
CMD [ "bash" ]

