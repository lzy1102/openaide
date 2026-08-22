/**
 * Agent 基准测试 —— 对齐 Go 版 eval/tasks.go 的题型设计。
 * 用法: npx tsx scripts/eval.ts [--quick]
 *
 * 每题: 通过 kernel.processStream 跑真实 agent,校验 MustContain/MustNotContain,
 * 记录 token 与耗时。评分: pass = 包含全部 MustContain 且不含任何 MustNotContain。
 */
import { buildApp } from '../packages/cli/src/app.js';
import { loadConfig } from '@openaide/config';
import { StreamChunkType } from '@openaide/core';

interface BenchTask {
  id: string;
  name: string;
  category: string;
  difficulty: 'easy' | 'medium' | 'hard';
  query: string;
  mustContain: string[];
  mustNotContain?: string[];
}

const TASKS: BenchTask[] = [
  // ── Easy ──
  {
    id: 'hello-response', name: 'Greeting Response',
    category: 'general', difficulty: 'easy',
    query: 'Hello! What can you help me with? (one sentence)',
    mustContain: ['help'],
    mustNotContain: ['error', 'failed'],
  },
  {
    id: 'simple-calc', name: 'Binary Search Complexity',
    category: 'coding', difficulty: 'easy',
    query: 'What is the time complexity of binary search? Answer in one sentence.',
    mustContain: ['O(log n)'],
  },
  {
    id: 'explain-react', name: 'Explain ReAct Loop',
    category: 'teaching', difficulty: 'easy',
    query: 'Explain the ReAct pattern used in this project (think → tool call → observe). Two sentences max.',
    mustContain: ['tool'],
  },

  // ── Medium ──
  {
    id: 'analyze-structure', name: 'Analyze Project Structure',
    category: 'research', difficulty: 'medium',
    query: 'List the main packages under packages/ in this project and their responsibilities briefly.',
    mustContain: ['core', 'llm'],
    mustNotContain: ["I don't know"],
  },
  {
    id: 'count-packages', name: 'Count Packages (tool use)',
    category: 'research', difficulty: 'medium',
    query: 'How many packages are in the packages/ directory? Use list_directory to check, then answer with just the number and the package names.',
    mustContain: ['8'],
  },
  {
    id: 'read-and-report', name: 'Read File & Report (tool use)',
    category: 'research', difficulty: 'medium',
    query: "Read package.json at the project root and tell me the project version number. Just the version.",
    mustContain: ['0.3'],
  },

  // ── Hard ──
  {
    id: 'multi-step-file', name: 'Multi-step: Find & Summarize (tool use)',
    category: 'coding', difficulty: 'hard',
    query: "Find the ReAct loop implementation file under packages/core/src/, read it, and summarize how one round works in 2 sentences. Mention the file name.",
    mustContain: ['react'],
  },
  {
    id: 'rate-limiter-design', name: 'Design Rate Limiter',
    category: 'coding', difficulty: 'hard',
    query: 'Design a rate limiter using the token bucket algorithm. Describe the key components. Keep it under 150 words.',
    mustContain: ['token', 'bucket'],
  },
];

interface TaskResult {
  task: BenchTask;
  passed: boolean;
  missing: string[];
  forbidden: string[];
  tokens: number;
  rounds: number;
  durationMs: number;
  response: string;
}

async function runTask(app: Awaited<ReturnType<typeof buildApp>>, task: BenchTask): Promise<TaskResult> {
  const t0 = Date.now();
  let content = '';
  let tokens = 0;
  let rounds = 0;

  // 免费模型偶发 500:指数退避重试最多 3 次
  let lastErr: unknown = null;
  for (let attempt = 1; attempt <= 3; attempt++) {
    try {
      for await (const chunk of app.kernel.processStream({
        sessionId: undefined,
        projectId: 'eval',
        userId: 'bench',
        content: task.query,
        options: {},
      })) {
        if (chunk.type === StreamChunkType.Content && chunk.content) content += chunk.content;
        if (chunk.type === StreamChunkType.Done) {
          if (chunk.usage) tokens = chunk.usage.totalTokens ?? 0;
          if (chunk.round) rounds = chunk.round;
        }
      }
      lastErr = null;
      break;
    } catch (err) {
      lastErr = err;
      content = '';
      if (attempt < 3) {
        const wait = attempt * 5000;
        process.stdout.write(`  (retry ${attempt}/3 after ${wait / 1000}s — ${String(err).slice(0, 60)})\n`);
        await new Promise((r) => setTimeout(r, wait));
      }
    }
  }
  if (lastErr) throw lastErr;

  const durationMs = Date.now() - t0;
  const lower = content.toLowerCase();
  const missing = task.mustContain.filter((s) => !lower.toLowerCase().includes(s.toLowerCase()));
  const forbidden = (task.mustNotContain ?? []).filter((s) => lower.toLowerCase().includes(s.toLowerCase()));

  return {
    task, passed: missing.length === 0 && forbidden.length === 0,
    missing, forbidden, tokens, rounds, durationMs, response: content,
  };
}

async function main() {
  const quick = process.argv.includes('--quick');
  const tasks = quick ? TASKS.filter((t) => t.difficulty === 'easy') : TASKS;

  console.log(`Agent benchmark — ${tasks.length} tasks\n${'='.repeat(60)}`);
  const app = await buildApp(await (await import('@openaide/config')).loadConfig());

  const results: TaskResult[] = [];
  for (const task of tasks) {
    process.stdout.write(`\n▶ [${task.difficulty}] ${task.id}: ${task.query.slice(0, 70)}...\n`);
    try {
      const r = await runTask(app, task);
      results.push(r);
      const mark = r.passed ? '✓ PASS' : '✗ FAIL';
      console.log(`  ${mark} | ${r.tokens} tok | ${(r.durationMs / 1000).toFixed(1)}s | ${r.rounds} rounds`);
      if (!r.passed) {
        if (r.missing.length) console.log(`  missing: ${r.missing.join(', ')}`);
        if (r.forbidden.length) console.log(`  forbidden found: ${r.forbidden.join(', ')}`);
      }
    } catch (err) {
      results.push({ task, passed: false, missing: [], forbidden: [String(err)], tokens: 0, rounds: 0, durationMs: 0, response: '' });
      console.log(`  ✗ ERROR: ${String(err).slice(0, 100)}`);
    }
  }

  console.log(`\n${'='.repeat(60)}\nSUMMARY`);
  const passedN = results.filter((r) => r.passed).length;
  const totalTok = results.reduce((a, r) => a + r.tokens, 0);
  for (const r of results) {
    console.log(`  ${r.passed ? '✓' : '✗'} ${r.task.id.padEnd(20)} ${String(r.task.difficulty).padEnd(7)} ${r.tokens} tok  ${(r.durationMs / 1000).toFixed(1)}s`);
  }
  console.log(`\nPass rate: ${passedN}/${results.length} (${Math.round((passedN / results.length) * 100)}%)`);
  console.log(`Total tokens: ${totalTok}`);
  process.exit(0);
}

main().catch((e) => { console.error('FATAL:', e); process.exit(1); });
