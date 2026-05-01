# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -ldflags '-w -s' -o /openaide-server ./src

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata sqlite

WORKDIR /app

COPY --from=builder /openaide-server /app/
COPY --from=builder /app/config.example.json /app/config.example.json

RUN mkdir -p /opt/openaide/data/db /opt/openaide/data/vectors /opt/openaide/data/ledis /opt/openaide/data/sessions /opt/openaide/logs

ENV TZ=Asia/Shanghai
ENV OPENAIDE_HOME=/opt/openaide

EXPOSE 19375

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:19375/health || exit 1

CMD ["/app/openaide-server"]
