/**
 * 内置工具集 —— 以插件形态导出（体现“一切皆插件”）。
 * 通过 @openaide/plugins 的动态加载器注册进内核，与用户插件无差别。
 */
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { readFile, readdir, writeFile, stat } from 'node:fs/promises';
import { join } from 'node:path';
import type { OpenAIDePlugin } from '@openaide/plugins';

const execFileAsync = promisify(execFile);

/**
 * 控制台输出智能解码：严格 UTF-8 校验失败即按 GBK 解码（Windows 中文环境 cmd/powershell
 * 的管道输出默认 GBK；chcp 65001 对部分内建命令不生效，实测 echo 即是）。
 */
function decodeConsoleOutput(buf: unknown): string {
  if (!buf) return '';
  const bytes = Buffer.isBuffer(buf) ? buf : Buffer.from(String(buf), 'utf8');
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
  } catch {
    try {
      return new TextDecoder('gbk').decode(bytes);
    } catch {
      return bytes.toString('utf8'); // 无 ICU 时兜底
    }
  }
}

/** 危险命令黑名单 —— 阻止 Agent 执行破坏性/危险操作 */
const BLOCKED_CMDS = new Set([
  'rm', 'rmdir', 'del', 'erase', // 删除
  'shutdown', 'reboot', 'poweroff', 'halt', // 关机
  'mkfs', 'fdisk', 'format', // 格式化
  'dd', // 磁盘
  'git reset --hard', 'git clean', // 破坏性 git
  'curl ... | sh', 'wget ... | sh', // 管道安装
]);

export const builtinToolsPlugin: OpenAIDePlugin = {
  name: 'builtin',
  version: '0.3.0',
  description: '内置文件/命令工具',
  category: 'capability',
  tools: [
    {
      name: 'read_file',
      description: '读取文件内容。file_path 必填，offset/limit 可选（行号从 1 开始）。',
      parameters: {
        type: 'object',
        properties: {
          file_path: { type: 'string', description: '要读取的文件绝对路径' },
          offset: { type: 'integer', description: '起始行号（默认 1）' },
          limit: { type: 'integer', description: '读取行数上限（默认全部）' },
        },
        required: ['file_path'],
      },
      async handler(args) {
        const filePath = args.file_path as string;
        if (!filePath) return { content: '', error: 'file_path required', errorCode: 'INVALID_ARGS' };
        try {
          const content = await readFile(filePath, 'utf8');
          const lines = content.split('\n');
          const offset = Math.max(1, (args.offset as number) ?? 1);
          const limit = args.limit as number | undefined;
          const end = limit ? offset + limit : lines.length;
          const slice = lines.slice(offset - 1, end);
          return { content: slice.join('\n') };
        } catch (err) {
          return { content: '', error: (err as Error).message, errorCode: 'NOT_FOUND' };
        }
      },
    },
    {
      name: 'list_directory',
      description: '列出目录内容。path 必填，返回文件/子目录名列表。',
      parameters: {
        type: 'object',
        properties: {
          path: { type: 'string', description: '要列出的目录绝对路径' },
        },
        required: ['path'],
      },
      async handler(args) {
        const dir = (args.path as string) ?? '.';
        try {
          const entries = await readdir(dir, { withFileTypes: true });
          const lines = entries
            .sort((a, b) => (a.isDirectory() === b.isDirectory() ? a.name.localeCompare(b.name) : a.isDirectory() ? -1 : 1))
            .map((e) => (e.isDirectory() ? `${e.name}/` : e.name));
          return { content: lines.join('\n') };
        } catch (err) {
          return { content: '', error: (err as Error).message, errorCode: 'NOT_FOUND' };
        }
      },
    },
    {
      name: 'write_file',
      description:
        '将完整内容写入文件（覆盖）。content 为文件的完整最终内容——不要用 shell echo/heredoc 写文件,直接用本工具避免转义错误。',
      parameters: {
        type: 'object',
        properties: {
          file_path: { type: 'string', description: '要写入的文件路径' },
          content: { type: 'string', description: '文件的完整最终内容' },
        },
        required: ['file_path', 'content'],
      },
      async handler(args) {
        const filePath = args.file_path as string;
        const content = args.content as string;
        if (!filePath || content === undefined || content === null) {
          return { content: '', error: 'file_path and content required', errorCode: 'INVALID_ARGS' };
        }
        try {
          await writeFile(filePath, content, 'utf8');
          // 写后回读验证(ACI):确认落盘内容与预期一致
          const back = await readFile(filePath, 'utf8');
          if (back !== content) {
            return { content: '', error: 'write verification failed: content mismatch', errorCode: 'VERIFY_FAILED' };
          }
          const lines = content.split('\n').length;
          return { content: `✓ wrote ${filePath} (${content.length} chars, ${lines} lines)` };
        } catch (err) {
          return { content: '', error: (err as Error).message, errorCode: 'WRITE_FAILED' };
        }
      },
    },
    {
      name: 'execute_command',
      description: '执行 shell 命令并返回 stdout/stderr。危险命令会被黑名单拦截。注意:写文件请用 write_file 工具,不要用 echo/重定向(转义易错)。',
      parameters: {
        type: 'object',
        properties: {
          command: { type: 'string', description: '要执行的命令' },
          cwd: { type: 'string', description: '工作目录（默认当前目录）' },
        },
        required: ['command'],
      },
      async handler(args) {
        const command = (args.command as string) ?? '';
        if (!command.trim()) return { content: '', error: 'command required', errorCode: 'INVALID_ARGS' };
        if (isBlocked(command)) {
          return { content: '', error: `command blocked by safety blacklist: ${command}`, errorCode: 'PERMISSION_DENIED' };
        }
        const isWin = process.platform === 'win32';
        const finalCommand = isWin ? `chcp 65001>nul && ${command}` : command;
        try {
          // 以 buffer 捕获后智能解码：Windows 控制台输出可能是 GBK（chcp 对部分内建命令不生效），
          // 严格 UTF-8 校验失败即回退 GBK，双保险覆盖中英文环境。
          // windowsVerbatimArguments：Node 默认把内层 " 转义成 \"（Unix 约定），cmd.exe 不认，
          // 导致含双引号的命令（powershell -Command "..." 等）被拆坏——改为整体手工加引号直传。
          const { stdout, stderr } = await execFileAsync(
            isWin ? 'cmd.exe' : 'sh',
            isWin ? ['/d', '/s', '/c', `"${finalCommand}"`] : ['-c', command],
            {
              cwd: (args.cwd as string) || undefined,
              maxBuffer: 4 * 1024 * 1024,
              encoding: 'buffer' as never,
              windowsVerbatimArguments: isWin,
            },
          );
          const out = [decodeConsoleOutput(stdout), decodeConsoleOutput(stderr)].filter(Boolean).join('\n');
          return { content: out || '(no output)' };
        } catch (err) {
          const e = err as { stdout?: Buffer; stderr?: Buffer; message: string };
          const detail = [decodeConsoleOutput(e.stdout), decodeConsoleOutput(e.stderr)].filter(Boolean).join('\n');
          return { content: detail || '', error: `exit error: ${e.message}`, errorCode: 'EXEC_FAILED' };
        }
      },
    },
  ],
};

function isBlocked(command: string): boolean {
  const first = command.trim().split(/[\s|&;]+/)[0]?.toLowerCase() ?? '';
  if (BLOCKED_CMDS.has(first)) return true;
  // 宽松匹配：命令片段含危险前缀
  for (const b of BLOCKED_CMDS) {
    if (command.toLowerCase().includes(b.toLowerCase())) return true;
  }
  return false;
}

export default builtinToolsPlugin;
