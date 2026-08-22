/**
 * Aider polyglot-benchmark harness — OpenAIDE pass@1 测试。
 *
 * 用法:
 *   npx tsx scripts/polyglot-eval.ts                 # 跑全部 10 题
 *   npx tsx scripts/eval-polyglot.ts proverb wordy   # 跑指定题
 *
 * 流程 (每题):
 *   1. 在 eval-workspace/<exercise>/ 准备 stub + 测试文件
 *   2. 以该目录为 cwd 启动 openaide one-shot:"实现 <file> 使 pytest 通过"
 *   3. agent 完成后跑官方 pytest 判定 pass/fail
 *   4. 统计 pass@1、token、耗时
 */
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';

const BENCH_ROOT = '/tmp/opencode/polyglot-benchmark/python/exercises/practice';
const WORKSPACE = join(process.cwd(), 'eval-workspace');
const TSX = 'npx tsx packages/cli/src/main.ts';

/** 10 道选题:覆盖字符串/类/逻辑/随机等不同能力 */
const EXERCISES = [
  'proverb',
  'wordy',
  'grade-school',
  'phone-number',
  'beer-song',
  'robot-name',
  'list-ops',
  'pig-latin',
  'hangman',
  'scale-generator',
];

interface TaskResult {
  exercise: string;
  passed: boolean;
  durationMs: number;
  agentOutput: string;
  pytestOutput: string;
}

function prepareWorkspace(exercise: string): string {
  const src = join(BENCH_ROOT, exercise);
  const dst = join(WORKSPACE, exercise);
  if (existsSync(dst)) rmSync(dst, { recursive: true });
  mkdirSync(dst, { recursive: true });
  // 目录名 kebab-case → 模块名 snake_case(如 grade-school → grade_school.py)
  const module = exercise.replace(/-/g, '_');
  for (const f of [`${module}.py`, `${module}_test.py`]) {
    cpSync(join(src, f), join(dst, f));
  }
  return dst;
}

function runAgent(dir: string, module: string): { output: string; ok: boolean } {
  const prompt =
    `Implement the functions in ${module}.py so that all tests in ${module}_test.py pass. ` +
    `The test file is authoritative — read it first. You may run ` +
    `\`python3 -m pytest ${module}_test.py -q\` to verify, and iterate until green. ` +
    `Do NOT modify the test file. Work in the current directory.`;
  // 关键:cwd = 题目目录(agent 的文件工具用相对路径);
  // CLI 用绝对路径 + 项目根的 tsx 二进制启动
  const projectRoot = process.cwd();
  const r = spawnSync(join(projectRoot, 'node_modules/.bin/tsx'), [join(projectRoot, 'packages/cli/src/main.ts'), prompt], {
    cwd: dir,
    encoding: 'utf8',
    timeout: 600_000,
    env: { ...process.env },
  });
  return { output: (r.stdout ?? '') + (r.stderr ?? ''), ok: r.status === 0 };
}

function runPytest(dir: string, module: string): { passed: boolean; output: string } {
  const r = spawnSync('python3', ['-m', 'pytest', `${module}_test.py`, '-q', '--tb=no'], {
    cwd: dir,
    encoding: 'utf8',
    timeout: 120_000,
  });
  return { passed: r.status === 0, output: (r.stdout ?? '') + (r.stderr ?? '').slice(-500) };
}

async function main() {
  const requested = process.argv.slice(2);
  const exercises = requested.length > 0 ? requested : EXERCISES;

  mkdirSync(WORKSPACE, { recursive: true });
  console.log(`Polyglot benchmark — ${exercises.length} exercises\n${'='.repeat(60)}`);

  const results: TaskResult[] = [];
  for (const ex of exercises) {
    const dir = prepareWorkspace(ex);
    const module = ex.replace(/-/g, '_');
    console.log(`\n▶ ${ex}`);
    const t0 = Date.now();

    const agent = runAgent(dir, module);
    if (!agent.ok) {
      console.log(`  ⚠ agent exited non-zero (continuing to pytest anyway)`);
    }

    const pytest = runPytest(dir, module);
    const durationMs = Date.now() - t0;
    results.push({ exercise: ex, passed: pytest.passed, durationMs, agentOutput: agent.output.slice(-300), pytestOutput: pytest.output });

    console.log(`  ${pytest.passed ? '✓ PASS' : '✗ FAIL'} | ${(durationMs / 1000).toFixed(1)}s`);
    if (!pytest.passed) {
      console.log(`  pytest: ${pytest.output.split('\n').slice(-3).join(' | ')}`);
    }
  }

  console.log(`\n${'='.repeat(60)}\nSUMMARY (pass@1)`);
  for (const r of results) {
    console.log(`  ${r.passed ? '✓' : '✗'} ${r.exercise.padEnd(18)} ${(r.durationMs / 1000).toFixed(1)}s`);
  }
  const n = results.filter((r) => r.passed).length;
  console.log(`\npass@1: ${n}/${results.length} (${Math.round((n / results.length) * 100)}%)`);
  process.exit(0);
}

main();
