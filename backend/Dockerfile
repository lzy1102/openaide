# Build stage
FROM golang:1.22-alpine AS builder

# 安装构建依赖
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 复制 go.mod 和 go.sum 先下载依赖（利用缓存）
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -ldflags '-w -s -extldflags "-static"' -o /openaide-server ./src

# Runtime stage
FROM alpine:3.19

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata sqlite

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /openaide-server /app/
COPY --from=builder /app/config.example.json /app/config.example.json

# 创建数据目录
RUN mkdir -p /app/data

# 设置时区
ENV TZ=Asia/Shanghai

# 暴露端口
EXPOSE 19375

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:19375/health || exit 1

# 启动服务
CMD ["/app/openaide-server"]
