### 6.1 存储选路、身份与格式取舍

```
resolveProjectWorkspace(): cwd 向上探测 .openaide/（跳过家目录层，防止全局配置目录被误当工作区）
  ├── 命中 → 复用（子目录启动自动归位到项目根）
  ├── 未命中 → 在 cwd 创建
  └── 家目录护栏：cwd ≡ home → 回退全局数据目录；workspace: off → 全局 SQLite
resolveIdentity(): OPENAIDE_USER > git user.name > email 前缀 > OS 用户名 > 随机兜底(持久化)
  └── 会话按身份分目录 sessions/<id>/、memory/<id>/ —— 多人各写各路径，结构上无合并冲突
      身份实时解析 + 单一旧目录自动改名收敛（多人目录并存时绝不吞并）
```

**格式取舍**（约束 = git 同步）：
- 可 diff 可审查：`git log -p` 直接看每轮对话增量
- 一个会话一个文件：merge 冲突面缩到单会话；坏文件只丢一个会话不殃及全库
- updatedAt 写进内容而非 mtime：克隆到新机器排序依然正确
- sessions 用 JSON（整体快照、全量重写），memory 用 JSONL（append-only 匹配增量写入）

**自动同步**（`session_sync: commit|push|off`）：每轮结束后提交 `.openaide/`（pathspec 限定，
不触碰用户暂存区）；未配置 `user.name` 时经 `-c` 注入解析出的兜底身份；30s 防抖，
CLI 退出前 flush（stdio 子进程会钉住事件循环）。`knowledge/*.md` 为团队共享区，
经 knowledge 拦截器注入 L1（mtime 缓存）。

SQLite 驱动经 `memory/sqlite-driver.ts` 适配双形态：Node 走 better-sqlite3，
Bun 单文件二进制走内置 bun:sqlite（better-sqlite3 的 bindings 运行时探测在
编译产物虚拟 FS 中必然失败）。该形态服务于 `workspace: off` 与 API server。

### 6.2 上下文压缩：LLM 摘要、失败上抛

```
reactLoop 每轮检查: estimateTokens > 90% × max_tokens
  └→ compressToBudget(): 渐进压缩 ≤3 次直到进入预算
       ├── system 消息全量保留（字节级稳定 → 前缀缓存不失效）
       ├── 最近 keep_recent 条（默认 12，kernel.compress 可配）原样保留
       ├── 更早历史 → LLM 结构化摘要（[User Intent]/[Key Facts]/[Current State]/[Notes]）
       └── LLM 失败/空摘要 → 上抛，本轮放弃压缩，下轮重试（无截断兜底）
```

**取舍**：摘要质量直接决定长任务可持续性，截断兜底会以"看似成功"的方式丢失
任务状态，比超预算更危险——宁可本轮带着超预算上下文请求（多数网关按实际
容量处理），等 LLM 可用再压缩。`retries: -1` 时瞬态故障在 provider 层已被
无限重试吸收，上抛仅发生在不可重试错误或用户取消。

---
