import { execFileSync } from 'node:child_process';
const GH = process.argv[2]!;
const H = ['--header', `authorization: Bearer ${GH}`];
const deadline = Date.now() + 14 * 60_000;
let runId: number | null = null;

while (Date.now() < deadline) {
  if (!runId) {
    const runs = JSON.parse(execFileSync('curl', ['-s', ...H, 'https://api.github.com/repos/lzy1102/openaide/actions/runs?per_page=3'], { maxBuffer: 10e6 }).toString());
    const run = (runs.workflow_runs as any[]).find((r) => r.head_branch === 'v0.3.2' && r.name === 'CI');
    if (run) runId = run.id;
  } else {
    const jobs = JSON.parse(execFileSync('curl', ['-s', ...H, `https://api.github.com/repos/lzy1102/openaide/actions/runs/${runId}/jobs`], { maxBuffer: 20e6 }).toString());
    const lines = (jobs.jobs as any[]).map((j) => `${j.name}:${j.status}${j.conclusion ? '/' + j.conclusion : ''}`);
    console.log(new Date().toISOString().slice(11, 19), '|', lines.join(' | '));
    if ((jobs.jobs as any[]).every((j) => j.status === 'completed')) break;
  }
  await new Promise((r) => setTimeout(r, 30_000));
}
