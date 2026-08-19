/**
 * CLI 参数解析 —— 独立的纯函数模块（无副作用，便于测试）。
 * 支持：子命令、--model、--output json、文件作为上下文。
 */
import { readFileSync, existsSync, statSync } from 'node:fs';

/** 解析后的命令行参数 */
export interface CliArgs {
  /** 子命令（repl/plugins/sessions/serve/setup/-c/--continue） */
  cmd?: string;
  /** --model <id> 覆盖 */
  model?: string;
  /** --output json */
  outputJson: boolean;
  /** 位置参数：存在的文件 → contextFiles，其余 → prompt */
  contextFiles: string[];
  prompt: string;
}

export function parseArgs(args: string[]): CliArgs {
  const out: CliArgs = { outputJson: false, contextFiles: [], prompt: '' };
  const positional: string[] = [];

  for (let i = 0; i < args.length; i++) {
    const a = args[i]!; // argv 不会含 undefined 元素
    if (a === '--model' || a === '-m') {
      if (i + 1 < args.length) out.model = args[++i];
      continue;
    }
    if (a.startsWith('--model=')) {
      out.model = a.slice('--model='.length);
      continue;
    }
    if (a === '--output') {
      if (i + 1 < args.length && args[i + 1] === 'json') {
        out.outputJson = true;
        i++;
      }
      continue;
    }
    if (a === '--output=json') {
      out.outputJson = true;
      continue;
    }
    if (a === 'repl' || a === 'plugins' || a === 'sessions' || a === 'serve' || a === 'setup' || a === '-c' || a === '--continue') {
      out.cmd = a;
      continue;
    }
    if (a === '--help' || a === '-h' || a === 'help' || a === '--version' || a === '-v' || a === 'version') {
      out.cmd = a;
      continue;
    }
    positional.push(a);
  }

  // 位置参数：存在的文件 → 上下文；其余 → prompt
  for (const p of positional) {
    if (existsSync(p) && statSync(p).isFile()) {
      out.contextFiles.push(p);
    } else {
      out.prompt = out.prompt ? `${out.prompt} ${p}` : p;
    }
  }
  return out;
}

/** 读取上下文文件并拼进 prompt */
export function buildPrompt(contextFiles: string[], prompt: string): string {
  const parts: string[] = [];
  for (const path of contextFiles) {
    try {
      const data = readFileSync(path, 'utf8');
      parts.push(`# 文件: ${path}\n\`\`\`\n${data}\n\`\`\``);
    } catch (err) {
      console.error(`[warn] read file ${path}:`, (err as Error).message);
    }
  }
  if (parts.length === 0) return prompt;
  return parts.join('\n\n') + (prompt ? `\n\n${prompt}` : '');
}
