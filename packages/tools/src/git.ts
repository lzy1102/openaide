/**
 * git —— 结构化 Git 工具（减少 token 浪费，返回 JSON 而非原始文本）
 */
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import type { OpenAIDePlugin } from '@openaide/plugins';

const execFileAsync = promisify(execFile);

async function git(cwd: string | undefined, args: string[]): Promise<{ stdout: string; stderr: string }> {
  try {
    const { stdout, stderr } = await execFileAsync('git', args, {
      cwd,
      encoding: 'utf8',
      maxBuffer: 10 * 1024 * 1024,
    } as any);
    return { stdout: String(stdout ?? ''), stderr: String(stderr ?? '') };
  } catch (e: any) {
    // git exits non-zero for no repo etc — still return stderr
    if (e.stdout !== undefined || e.stderr !== undefined) {
      return { stdout: String(e.stdout ?? ''), stderr: String(e.stderr ?? '') };
    }
    throw e;
  }
}

function parseStatus(porcelain: string) {
  return porcelain
    .split('\n')
    .filter(Boolean)
    .map(line => {
      const x = line[0];
      const y = line[1];
      const path = line.slice(3);
      let status = 'modified';
      if (x === '?' && y === '?') status = 'untracked';
      else if (x === 'A' || y === 'A') status = 'added';
      else if (x === 'D' || y === 'D') status = 'deleted';
      else if (x === 'R' || y === 'R') status = 'renamed';
      else if (x === 'M' || y === 'M') status = 'modified';
      return { path, status, x, y };
    });
}

export const gitPlugin: OpenAIDePlugin = {
  name: 'git',
  version: '0.3.0',
  description: '结构化 Git 工具',
  category: 'capability',
  tools: [
    {
      name: 'git_status',
      description: '获取 git 状态，返回分支、未提交文件列表（JSON）',
      parameters: {
        type: 'object',
        properties: {
          cwd: { type: 'string', description: '工作目录' },
        },
      },
      handler: async (args) => {
        const cwd = (args.cwd as string) || undefined;
        const { stdout: branchOut } = await git(cwd, ['rev-parse', '--abbrev-ref', 'HEAD']);
        const branch = branchOut.trim() || 'unknown';
        // check if repo
        const { stderr: errCheck } = await git(cwd, ['rev-parse', '--git-dir']);
        if (errCheck.includes('not a git repository')) {
          return { content: '', error: 'not a git repository', errorCode: 'NOT_FOUND' };
        }
        const { stdout } = await git(cwd, ['status', '--porcelain=v1', '--branch']);
        // porcelain v1 --branch adds first line ## branch...oid
        const lines = stdout.split('\n');
        const filesRaw = lines.filter(l => !l.startsWith('##')).join('\n');
        const files = parseStatus(filesRaw);
        return { content: JSON.stringify({ branch, files }, null, 2) };
      },
    },
    {
      name: 'git_diff',
      description: '获取 git diff，支持 staged/文件过滤，返回 JSON',
      parameters: {
        type: 'object',
        properties: {
          cwd: { type: 'string', description: '工作目录' },
          staged: { type: 'boolean', description: '是否只看已暂存的变更' },
          file: { type: 'string', description: '限定单个文件' },
        },
      },
      handler: async (args) => {
        const cwd = (args.cwd as string) || undefined;
        const staged = args.staged as boolean | undefined;
        const file = args.file as string | undefined;
        const gitArgs = ['diff', '--no-color'];
        if (staged) gitArgs.push('--staged');
        // unified 3 lines context, enough for model
        gitArgs.push('-U3');
        if (file) gitArgs.push('--', file);
        const { stdout, stderr } = await git(cwd, gitArgs);
        if (stderr && !stdout) {
          // not a git repo or other error
          if (stderr.includes('not a git repository')) {
            return { content: '', error: 'not a git repository', errorCode: 'NOT_FOUND' };
          }
        }
        const truncated = stdout.length > 20000 ? stdout.slice(0, 20000) + '\n[...truncated]' : stdout;
        return { content: JSON.stringify({ diff: truncated || '(no changes)', staged: !!staged }, null, 2) };
      },
    },
    {
      name: 'git_log',
      description: '获取 git 提交历史',
      parameters: {
        type: 'object',
        properties: {
          cwd: { type: 'string', description: '工作目录' },
          count: { type: 'number', description: '返回条数，默认 10' },
        },
      },
      handler: async (args) => {
        const cwd = (args.cwd as string) || undefined;
        const count = Math.min(Math.max(Number(args.count ?? 10), 1), 50);
        const { stdout, stderr } = await git(cwd, ['log', `--oneline`, `-n`, String(count), '--decorate']);
        if (stderr.includes('not a git repository')) {
          return { content: '', error: 'not a git repository', errorCode: 'NOT_FOUND' };
        }
        const commits = stdout
          .split('\n')
          .filter(Boolean)
          .map(line => {
            const firstSpace = line.indexOf(' ');
            return { hash: line.slice(0, firstSpace), message: line.slice(firstSpace + 1) };
          });
        return { content: JSON.stringify({ commits }, null, 2) };
      },
    },
  ],
};
