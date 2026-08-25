import { execFileSync } from 'node:child_process';

const GH = process.argv[2]!;
const H = { authorization: `Bearer ${GH}`, accept: 'application/vnd.github+json', 'user-agent': 'monitor' };
const g = (path: string): any =>
  JSON.parse(execFileSync('curl', ['-s', '-H', `authorization: Bearer ${GH}`, '-H', 'accept: application/vnd.github+json', 'user-agent: monitor', `https://api.github.com${path}`], { maxBuffer: 20 * 1024 * 1024 }).toString());

const deadline = Date.now() + 16 * 60_000;
let runId: number | null = null;

while (Date.now() < deadline) {
  if (!runId) {
    const runs = g('/repos/lzy1102/openaide/actions/runs?per_page=5');
    const run = (runs.workflow_runs ?? []).find((r: any) => r.head_branch === 'v0.3.2');
    if (run) { runId = run.id; console.log(`run ${run.id} (${run.status})`); }
  } else {
    const jobs = g(`/repos/lzy1102/openaide/actions/runs/${runId}/jobs`);
    const lines = (jobs.jobs ?? []).map((j: any) => `${j.name}:${j.status}${j.conclusion ? '/' + j.conclusion : ''}`);
    console.log(new Date().toISOString().slice(11, 19), '|', lines.join(' | '));
    if ((jobs.jobs ?? []).every((j: any) => j.status === 'completed')) {
      console.log('\n=== COMPLETED ===');
      for (const j of jobs.jobs as any[]) console.log(`${j.name} -> ${j.conclusion}`);
      break;
    }
  }
  await new Promise((r) => setTimeout(r, 30_000));
}
