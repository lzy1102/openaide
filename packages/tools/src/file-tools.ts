/**
 * file-tools 插件 —— 文件搜索与精确编辑。
 *
 * 独立于 builtin(读/列/命令),以插件形态提供写侧与检索能力:
 *  - grep_search: 递归正则搜索,带行号输出
 *  - diff_edit:   search/replace 块精确替换(全文唯一匹配才生效,防误改)
 */
import { readFile, writeFile } from 'node:fs/promises';
import { join, relative } from 'node:path';
import { readdirSync, statSync } from 'node:fs';
import type { OpenAIDePlugin } from '@openaide/plugins';

/** 递归收集文件(跳过常见噪声目录),返回绝对路径列表 */
function walkFiles(root: string, depth = 0, acc: string[] = []): string[] {
  const SKIP = new Set(['node_modules', '.git', 'dist', 'build', '.next', '__pycache__', 'eval-workspace']);
  if (depth > 8) return acc;
  let entries;
  try {
    entries = readdirSync(root, { withFileTypes: true });
  } catch {
    return acc;
  }
  for (const e of entries) {
    if (e.name.startsWith('.')) continue;
    const full = join(root, e.name);
    if (e.isDirectory()) {
      if (!SKIP.has(e.name)) walkFiles(full, depth + 1, acc);
    } else if (e.isFile()) {
      acc.push(full);
    }
  }
  return acc;
}

export const fileToolsPlugin: OpenAIDePlugin = {
  name: 'file-tools',
  version: '0.1.0',
  description: '文件检索(grep)与精确编辑(search/replace)',
  category: 'capability',
  tools: [
    {
      name: 'grep_search',
      description:
        '在目录下递归搜索文本(支持正则)。返回 file:line: 匹配行 列表。优先用本工具而非 execute_command grep。',
      parameters: {
        type: 'object',
        properties: {
          pattern: { type: 'string', description: '正则表达式' },
          path: { type: 'string', description: '搜索根目录(默认当前目录)' },
          include: { type: 'string', description: '文件名过滤 glob 片段,如 "*.ts"' },
        },
        required: ['pattern'],
      },
      async handler(args) {
        const pattern = String(args.pattern ?? '');
        const root = (args.path as string) || process.cwd();
        const include = args.include ? String(args.include) : undefined;
        if (!pattern) return { content: '', error: 'pattern required', errorCode: 'INVALID_ARGS' };

        let regex: RegExp;
        try {
          regex = new RegExp(pattern, 'i');
        } catch (err) {
          return { content: '', error: `invalid regex: ${(err as Error).message}`, errorCode: 'INVALID_ARGS' };
        }

        const files = walkFiles(root).filter((f) => !include || f.includes(include.replace(/\*/g, '')));
        const out: string[] = [];
        let truncated = false;
        for (const file of files) {
          if (out.length >= 200) { truncated = true; break; }
          try {
            if (statSync(file).size > 512 * 1024) continue; // 跳过 >512K 文件
            const content = await readFile(file, 'utf8');
            const lines = content.split('\n');
            for (let i = 0; i < lines.length; i++) {
              const line = lines[i] ?? '';
              if (regex.test(line)) {
                const rel = relative(root, file) || file;
                out.push(`${rel}:${i + 1}: ${line.trim().slice(0, 200)}`);
                if (out.length >= 200) { truncated = true; break; }
              }
            }
          } catch {
            // 二进制/无权限文件跳过
          }
        }
        if (out.length === 0) return { content: '(no matches)' };
        return { content: out.join('\n') + (truncated ? '\n... (results truncated at 200)' : '') };
      },
    },
    {
      name: 'diff_edit',
      description:
        '精确编辑文件:把 search 块替换为 replace 块。search 必须在文件中唯一出现(含足够上下文),否则拒绝执行防误改。',
      parameters: {
        type: 'object',
        properties: {
          file_path: { type: 'string', description: '目标文件路径' },
          search: { type: 'string', description: '要替换的原文块(需在文件中唯一)' },
          replace: { type: 'string', description: '替换后的新内容' },
        },
        required: ['file_path', 'search', 'replace'],
      },
      async handler(args) {
        const filePath = String(args.file_path ?? '');
        const search = String(args.search ?? '');
        const replace = String(args.replace ?? '');
        if (!filePath || !search) {
          return { content: '', error: 'file_path and search required', errorCode: 'INVALID_ARGS' };
        }
        let content: string;
        try {
          content = await readFile(filePath, 'utf8');
        } catch (err) {
          return { content: '', error: (err as Error).message, errorCode: 'NOT_FOUND' };
        }

        const count = content.split(search).length - 1;
        if (count === 0) {
          return { content: '', error: 'search block not found in file — read the file again and copy the exact text', errorCode: 'NOT_FOUND' };
        }
        if (count > 1) {
          return { content: '', error: `search block matches ${count} times — add more surrounding context to make it unique`, errorCode: 'AMBIGUOUS' };
        }

        const updated = content.replace(search, replace);
        await writeFile(filePath, updated, 'utf8');
        // 写后回读验证(ACI)
        const back = await readFile(filePath, 'utf8');
        if (back !== updated) {
          return { content: '', error: 'write verification failed', errorCode: 'VERIFY_FAILED' };
        }
        return { content: `✓ edited ${filePath} (1 block replaced)` };
      },
    },
  ],
};

export default fileToolsPlugin;
