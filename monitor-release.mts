/** 监控 v0.3.1 发布流水线：轮询 job 状态，失败时拉取日志尾部 */
import { execFileSync } from 'node:child_process';

const GH = process.argv[2]!;
const H = { authorization: `Bearer ${GH}`, accept: 'application/vnd.github+json', 'user-agent': 'monitor' };
const g = (path: string): any =>
  JSON.parse(execFileSync('curl', ['-s', '-H', `authorization: Bearer ${GH}`, '-H', 'accept: application/vnd.github+json', `-H`, 'user-agent: monitor', `https://api.github.com${path}`], { maxBuffer: 20 * 1024 * 1024 }).toString());

const deadline = Date.now() + 14 * 60_000;
let runId: number | null = null;

while (Date.now() < deadline) {
  if (!runId) {
    const runs = g('/repos/lzy1102/openaide/actions/runs?per_page=5');
    const run = (runs.workflow_runs ?? []).find((r: any) => r.head_branch === 'v0.3.1');
    if (run) { runId = run.id; console.log(`run ${run.id} found (${run.status})`); }
    else { console.log('waiting for run to appear...'); }
  } else {
    const jobs = g(`/repos/lzy1102/openaide/actions/runs/${runId}/jobs`);
    const lines = (jobs.jobs ?? []).map((j: any) => `${j.name}: ${j.status}${j.conclusion ? '/' + j.conclusion : ''}`);
    console.clear?.();
    console.log(new Date().toISOString().slice(11, 19), '|', lines.join(' | '));
    if ((jobs.jobs ?? []).every((j: any) => j.status === 'completed')) {
      console.log('\n=== PIPELINE COMPLETED ===');
      for (const j of jobs.jobs as any[]) {
        console.log(`\n## ${j.name} -> ${j.conclusion}`);
        if (j.conclusion !== 'success') {
          try {
            const log = execFileSync('curl', ['-sL', '-H', `authorization: Bearer ${GH}`, j.logs_url], { maxBuffer: 30 * 1024 * 1024 }).toString();
            const tail = log.split('\n').filter((l) => /error|ERR!|failed|✖|npm warn/i.test(l)).slice(-15);
            console.log(tail.join('\n'));
          } catch (e) { console.log('(log unavailable)', (e as Error).message.slice(0, 100)); }
        }
      }
      break;
    }
  }
  await new Promise((r) => setTimeout(r, 25_000));
}
if (!runId) console.log('TIMEOUT: run never appeared');
