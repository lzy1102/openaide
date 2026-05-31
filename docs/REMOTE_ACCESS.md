# 远程访问 · Remote Access

OpenAIDE 默认运行在本地，但通过以下方式可以从任何地方访问——**手机、平板、笔记本，随时随地编程。**

---

## 方案对比

| 方案 | 难度 | 费用 | 延迟 | 适用场景 |
|------|------|------|------|----------|
| Cloudflare Tunnel | ⭐ 最简单 | 免费（需域名） | 低 | 个人使用，快速上手 |
| frp 内网穿透 | ⭐⭐ 中等 | 需一台公网 VPS | 极低 | 有 VPS，追求速度 |
| 云服务器部署 | ⭐⭐⭐ 较难 | VPS 费用 | 极低 | 24/7 在线，团队使用 |
| Tailscale | ⭐ 最简单 | 免费 | 极低 | 个人设备间互联 |

---

## 方案一：Cloudflare Tunnel（推荐）

**优点**：免费、不需要公网 IP、自动 HTTPS、5 分钟配好。

### 1. 安装 cloudflared

```bash
# Linux/macOS
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o cloudflared
chmod +x cloudflared
sudo mv cloudflared /usr/local/bin/

# 或包管理器
brew install cloudflare/cloudflare/cloudflared     # macOS
sudo apt install cloudflared                        # Ubuntu/Debian
```

### 2. 创建隧道

```bash
cloudflared tunnel login      # 浏览器授权，选域名
cloudflared tunnel create openaide
```

### 3. 配置

创建 `~/.cloudflared/config.yml`:

```yaml
tunnel: <你的隧道 ID>
credentials-file: /home/<用户>/.cloudflared/<隧道 ID>.json

ingress:
  - hostname: ai.yourdomain.com
    service: http://localhost:8080
  - service: http_status:404
```

### 4. 启动

```bash
# 启动 OpenAIDE
openaide-server

# 另一个终端启动隧道
cloudflared tunnel run openaide
```

访问 `https://ai.yourdomain.com` 即可使用 Web 界面。

### 5. 后台运行

```bash
# systemd 服务 (自动启动)
sudo cloudflared service install

# 或 tmux/screen
tmux new -s openaide
openaide-server
# Ctrl+B D 分离
```

---

## 方案二：frp 内网穿透

**优点**：延迟极低，完全掌控，适合已有 VPS 的用户。

### 1. 服务器端（VPS）

```bash
# 下载 frp
wget https://github.com/fatedier/frp/releases/download/v0.61.0/frp_0.61.0_linux_amd64.tar.gz
tar xzf frp_*.tar.gz && cd frp_*

# 编辑 frps.toml
cat > frps.toml << EOF
bindPort = 7000
vhostHTTPPort = 8080
EOF

# 启动
./frps -c frps.toml
```

### 2. 客户端（你的电脑）

```bash
# 编辑 frpc.toml
cat > frpc.toml << EOF
serverAddr = "你的VPS的IP"
serverPort = 7000

[[proxies]]
name = "openaide"
type = "http"
localPort = 8080
customDomains = ["ai.yourdomain.com"]
EOF

# 启动
./frpc -c frpc.toml
```

访问 `http://你的VPS的IP:8080` 或配置的域名。

---

## 方案三：云服务器部署

**优点**：最稳定，24/7 运行，适合团队。

### 1. 购买 VPS

推荐：阿里云、腾讯云、AWS Lightsail、Hetzner（$5/月起）

### 2. 安装 OpenAIDE

```bash
# SSH 到 VPS
ssh root@你的服务器IP

# 一键安装
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash

# 配置
vi ~/.openaide/config.yaml
# 填入你的 API Key

# 启动
openaide-server
```

### 3. 配置 HTTPS（可选）

```bash
# 安装 Caddy
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudflare.com/public/caddy-stable/debian/any-version/latest.json' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update && sudo apt install caddy

# Caddy 自动申请 Let's Encrypt 证书
# /etc/caddy/Caddyfile
ai.yourdomain.com {
    reverse_proxy localhost:8080
}

sudo systemctl enable --now caddy
```

### 4. 后台运行

```bash
# systemd 服务
sudo tee /etc/systemd/system/openaide.service << EOF
[Unit]
Description=OpenAIDE Server
After=network.target

[Service]
Type=simple
User=$USER
ExecStart=$HOME/.openaide/bin/openaide-server
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now openaide
```

---

## 方案四：Tailscale

**优点**：零配置，P2P 加密，手机/电脑/平板自动互联。

### 安装

```bash
# 所有设备安装 Tailscale
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up
```

### 使用

```bash
# 启动 OpenAIDE
openaide-server

# 手机/平板打开 Tailscale，访问
# http://<电脑的Tailscale IP>:8080
```

---

## 手机端使用

所有方案配好后，手机浏览器打开你的地址即可：

```
https://ai.yourdomain.com
```

界面自动适配移动端，支持：

- 📝 对话编程：输入需求，Agent 远程执行
- 📂 文件浏览：查看项目文件结构
- 🔧 模型切换：在设置页面切换 LLM
- 📊 仪表盘：查看系统状态

### 快捷方式

**iOS**: Safari → 分享 → 添加到主屏幕  
**Android**: Chrome → 菜单 → 添加到主屏幕

---

## 安全建议

1. **启用 JWT 认证**

```bash
# config.yaml
OPENAIDE_AUTH=true
OPENAIDE_JWT_SECRET=your-random-secret
```

```bash
# 注册用户
curl -X POST https://ai.yourdomain.com/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

2. **使用 HTTPS**：Cloudflare Tunnel 和 Caddy 都自动提供
3. **限制 IP**：在 VPS 上用 `ufw` 防火墙
4. **定期更新**：`openaide update`

---

## 故障排查

| 问题 | 解决 |
|------|------|
| 无法访问 | 检查防火墙：`sudo ufw allow 8080` |
| 连接超时 | 检查 `openaide-server` 是否在运行 |
| HTTPS 证书错误 | `cloudflared tunnel cleanup` 然后重连 |
| 飞书机器人无响应 | 检查 `feishu.webhook_url` 配置 |
