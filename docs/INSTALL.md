# OpenAIDE 安装指南

## 系统要求

- **Go 1.26+** (源码编译) 或仅需 `curl` (二进制安装)
- **操作系统**: Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64)
- **依赖**: git (可选), 现代终端 (REPL 需要)

## 方法一: 一键安装 (推荐)

```bash
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
```

安装到 `~/.openaide/`，自动添加到 PATH。

指定版本:
```bash
VERSION=v1.0.0 curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
```

## 方法二: 源码编译

```bash
git clone https://github.com/lzy1102/openaide.git
cd openaide && make install
```

编译产物:
- `~/.openaide/bin/openaide` — 统一二进制 (CLI + 服务器)

## 方法三: 本地编译

如果已有源码:
```bash
cd openaide/backend
CGO_ENABLED=0 go build -o ~/.openaide/bin/openaide ./cmd/cli
```

## 初始化配置

```bash
openaide setup
```

交互式向导会引导你完成:
1. 语言选择 (中文/English)
2. LLM 提供商 (DeepSeek, OpenAI, Anthropic)
3. API Key
4. 模型选择

配置文件位置: `~/.openaide/config.yaml`

## 验证安装

```bash
openaide --version   # 显示版本
openaide "hello"     # 一次性测试
```

## 卸载

```bash
rm -rf ~/.openaide
# 如果添加了 PATH，从 ~/.bashrc 或 ~/.zshrc 中删除对应行
```
