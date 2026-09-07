/**
 * 平台环境附注 —— 启动时探测可用脚本运行时，注入 [Platform] 系统消息。
 *
 * 背景：Windows 上 agent 用 PowerShell/cmd 单行做复杂文本处理极易失败
 * （引号转义、$variable 展开、GBK 编码），每次失败都是完整一轮 token。
 * 附注引导模型：优先用专用工具（search_files/write_file），复杂处理写
 * Python/Node 脚本执行，而不是拼 PowerShell。
 *
 * 探测每个进程只做一次，附注字节级稳定——不破坏前缀缓存。
 */
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Interceptor } from '@openaide/core';

const execFileAsync = promisify(execFile);

export interface RuntimeInfo {
  /** 运行时名（python/node/…），execute_command 里实际敲的命令 */
  cmd: string;
  /** `--version` 首行输出 */
  version: string;
}

const PROBES: Array<{ label: string; cmd: string; args?: string[] }> = [
  { label: 'python', cmd: 'python' },
  { label: 'python3', cmd: 'python3' },
  { label: 'py', cmd: 'py', args: ['-3'] },
  { label: 'node', cmd: 'node' },
  { label: 'bun', cmd: 'bun' },
  { label: 'deno', cmd: 'deno' },
];

async function probe(entry: { cmd: string; args?: string[] }): Promise<RuntimeInfo | null> {
  try {
    const { stdout, stderr } = await execFileAsync(entry.cmd, entry.args ?? ['--version'], {
      timeout: 3000,
      windowsHide: true,
    });
    // Windows Store 的 python 占位 stub 把提示写到 stderr 且退出码非 0 —— 走 catch 丢弃
    const line = (String(stdout) || String(stderr)).split('\n')[0].trim();
    return line ? { cmd: entry.cmd, version: line } : null;
  } catch {
    return null;
  }
}

let cachePromise: Promise<RuntimeInfo[]> | null = null;

/** 探测可用脚本运行时（每进程一次，并发探测，单个失败不影响其余） */
export function detectRuntimes(): Promise<RuntimeInfo[]> {
  cachePromise ??= (async () => {
    const results = await Promise.all(PROBES.map(probe));
    // 同一运行时多入口（python/python3/py）只保留第一个命中的
    const seen = new Set<string>();
    return results.filter((r): r is RuntimeInfo => {
      if (!r) return false;
      const key = r.version.split(' ')[0].toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  })();
  return cachePromise;
}

/**
 * 生成平台附注文本。
 * @param platform  进程平台（测试可注入）
 * @param runtimes  探测结果（测试可注入）
 */
export async function buildPlatformNote(
  platform: NodeJS.Platform = process.platform,
  runtimes?: RuntimeInfo[],
): Promise<string> {
  const found = runtimes ?? (await detectRuntimes());
  if (platform !== 'win32') {
    // POSIX：只有运行时清单，无 Windows 特规
    if (found.length === 0) return '';
    return `[Platform]\n可用脚本运行时: ${found.map((r) => `${r.cmd} (${r.version})`).join(', ')}`;
  }

  const lines = [
    '[Platform]',
    '运行环境: Windows，shell 是 cmd.exe。没有 bash/grep/sed/awk/head/tail。',
    '- 文本搜索用 search_files，文件读写用 read_file/write_file，列目录用 list_directory——不要用 shell 做这些。',
    '- 不要用 powershell / powershell -Command 包装命令——转义规则繁琐、极易翻车；默认 cmd.exe 语法。',
    '- 复杂文本处理/批处理优先写脚本执行，不要拼 shell 单行：',
    '  1) 用 write_file 写脚本到临时目录（'
      + join(tmpdir(), 'openaide-script.py 或 .mjs') + '）',
    '  2) execute_command 运行它（如 python <脚本路径>）',
    '  3) 用完删除脚本',
  ];
  if (found.length > 0) {
    lines.push(`- 已检测到脚本运行时: ${found.map((r) => `${r.cmd} (${r.version})`).join(', ')}`);
  } else {
    lines.push('- 未检测到 python/node 等脚本运行时——任务限定在专用工具与简单 shell 命令内完成。');
  }
  return lines.join('\n');
}

/**
 * 平台附注拦截器：把 [Platform] 系统消息插入首条 system 之后。
 * 探测/组文只在首次调用发生，之后每轮注入相同字节——前缀缓存不受影响。
 */
export function createEnvironmentInterceptor(): Interceptor {
  let note: string | null = null;
  return {
    name: 'environment',
    async beforeLLM(info) {
      note ??= await buildPlatformNote();
      if (!note) return { action: 'allow' };
      const messages = [...info.messages];
      const sysIdx = messages.findIndex((m) => m.role === 'system');
      const platformMsg = { role: 'system' as const, content: note };
      if (sysIdx >= 0) messages.splice(sysIdx + 1, 0, platformMsg);
      else messages.unshift(platformMsg);
      return { action: 'modify', payload: messages };
    },
  };
}
