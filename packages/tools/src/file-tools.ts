/**
 * file-tools 插件 —— 文件搜索与精确编辑。
 *
 * 独立于 builtin(读/列/命令),以插件形态提供写侧与检索能力:
 *  - grep_search: 递归正则搜索,带行号输出
 *  - diff_edit:   search/replace 块精确替换(全文唯一匹配才生效,防误改)
 */
import { readFile, writeFile } from 'node:fs/promises';
import { basename, join, relative } from 'node:path';
import { readdirSync, statSync } from 'node:fs';
import type { OpenAIDePlugin } from '@openaide/plugins';

/**
 * 跳过的噪声目录。
 * 注意只跳过「目录」——旧实现连 .env / .gitignore / .github 全部跳过，
 * agent 想查配置文件永远查不到，只能退化去 shell 里翻。
 */
const SKIP_DIRS = new Set([
  'node_modules', '.git', '.svn', '.hg', 'dist', 'build', '.next', '.turbo',
  '.cache', '.venv', '__pycache__', 'eval-workspace',
]);

/** 递归收集文件(跳过噪声目录),返回绝对路径列表 */
function walkFiles(root: string, depth = 0, acc: string[] = []): string[] {
  if (depth > 8) return acc;
  let entries;
  try {
    entries = readdirSync(root, { withFileTypes: true });
  } catch {
    return acc;
  }
  for (const e of entries) {
    const full = join(root, e.name);
    if (e.isDirectory()) {
      if (!SKIP_DIRS.has(e.name)) walkFiles(full, depth + 1, acc);
    } else if (e.isFile()) {
      acc.push(full);
    }
  }
  return acc;
}

/**
 * include 过滤：支持 * / ? / ** 通配符（旧实现只是 `f.includes(glob去掉*)`，
 * "*.ts" 能凑合，但 "src/*.ts" / "**\/*.test.ts" 这类写法语义全错）。
 */
function includeMatcher(glob?: string): ((rel: string, base: string) => boolean) | undefined {
  if (!glob) return undefined;
  // 无通配符时按子串匹配（兼容 include: ".ts" 这类旧用法）
  if (!/[*?]/.test(glob)) {
    return (rel, base) => base.includes(glob) || rel.includes(glob);
  }
  const g = glob.replace(/\\/g, '/');
  let src = '';
  for (let i = 0; i < g.length; i++) {
    const ch = g[i]!;
    if (ch === '*') {
      if (g[i + 1] === '*') {
        src += '.*';
        i++;
        if (g[i + 1] === '/') i++; // "**/" 也匹配零层目录
      } else {
        src += '[^/]*';
      }
    } else if (ch === '?') {
      src += '[^/]';
    } else {
      src += ch.replace(/[.+^${}()|[\]\\]/g, '\\$&');
    }
  }
  const re = new RegExp(`^${src}$`, 'i');
  return (rel, base) => re.test(base) || re.test(rel);
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
        '在目录下递归搜索文本(支持正则)。返回 file:line: 匹配行 列表。优先用本工具,不要用 shell 的 grep(Windows 上没有)。',
      parameters: {
        type: 'object',
        properties: {
          pattern: { type: 'string', description: '正则表达式' },
          path: { type: 'string', description: '搜索根目录(默认当前目录)' },
          include: { type: 'string', description: '文件名过滤 glob,如 "*.ts" / "**/*.test.ts"' },
          max_results: { type: 'integer', description: '最多返回条数(默认 200,上限 1000)' },
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

        const matchesInclude = includeMatcher(include);
        const files = walkFiles(root).filter((f) => {
          if (!matchesInclude) return true;
          return matchesInclude(relative(root, f).replace(/\\/g, '/'), basename(f));
        });
        const limit = Math.min(Math.max(Math.floor(Number(args.max_results ?? 200)) || 200, 1), 1000);
        const out: string[] = [];
        let truncated = false;
        for (const file of files) {
          if (out.length >= limit) { truncated = true; break; }
          try {
            if (statSync(file).size > 512 * 1024) continue; // 跳过 >512K 文件
            const content = await readFile(file, 'utf8');
            if (content.includes(String.fromCharCode(0))) continue; // 二进制(utf8 解码不报错但内容无意义)
            const lines = content.split('\n');
            for (let i = 0; i < lines.length; i++) {
              const line = lines[i] ?? '';
              if (regex.test(line)) {
                const rel = relative(root, file) || file;
                out.push(`${rel}:${i + 1}: ${line.trim().slice(0, 200)}`);
                if (out.length >= limit) { truncated = true; break; }
              }
            }
          } catch {
            // 二进制/无权限文件跳过
          }
        }
        if (out.length === 0) return { content: '(no matches)' };
        return { content: out.join('\n') + (truncated ? `\n... (results truncated at ${limit})` : '') };
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

        const occurrences = (hay: string, needle: string): number => hay.split(needle).length - 1;

        // 行尾归一：模型给的 search 通常是 LF，Windows 文件常是 CRLF。
        // 不兼容的话每次编辑都报 not found，模型反复重读仍然失败。
        let needle = search;
        let replacement = replace;
        let count = occurrences(content, needle);
        if (count === 0) {
          const variants: Array<[string, string]> = [];
          if (content.includes('\r\n') && !needle.includes('\r\n')) {
            variants.push([needle.replace(/\r?\n/g, '\r\n'), replacement.replace(/\r?\n/g, '\r\n')]);
          }
          if (!content.includes('\r\n') && needle.includes('\r\n')) {
            variants.push([needle.replace(/\r\n/g, '\n'), replacement.replace(/\r\n/g, '\n')]);
          }
          for (const [n, r] of variants) {
            if (occurrences(content, n) > 0) {
              needle = n;
              replacement = r;
              count = occurrences(content, n);
              break;
            }
          }
        }

        if (count === 0) {
          return { content: '', error: 'search block not found in file — read the file again and copy the exact text', errorCode: 'NOT_FOUND' };
        }
        if (count > 1) {
          return { content: '', error: `search block matches ${count} times — add more surrounding context to make it unique`, errorCode: 'AMBIGUOUS' };
        }

        // 用 indexOf 切片而非 String.replace：字符串替换会把 replacement 里的
        // $& / $1 / $` / $' 当成特殊模式展开，改代码时（模板串、正则、shell 变量）
        // 会被静默改坏。
        const at = content.indexOf(needle);
        const updated = content.slice(0, at) + replacement + content.slice(at + needle.length);
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
