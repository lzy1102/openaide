# Build stage - 使用官方Go镜像编译
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 先复制依赖文件，利用Docker缓存层
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译二进制文件（静态链接，无CGO，减小体积）
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w -s -extldflags "-static"' -o /openaide-server ./cmd/server

# Runtime stage - 使用更小的alpine镜像
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# 复制编译好的二进制文件
COPY --from=builder /openaide-server /app/openaide-server

WORKDIR /app

ENV TZ=Asia/Shanghai
ENV OPENAIDE_HOME=/opt/openaide

EXPOSE 8080

# 创建非root用户运行（安全最佳实践）
RUN adduser -D -u 1000 openaide && \
    mkdir -p /opt/openaide/data /opt/openaide/logs && \
    chown -R openaide:openaide /opt/openaide

USER openaide:openaide

CMD ["/app/openaide-server"]
