import { execFileSync } from 'node:child_process';
const GH = process.argv[2]!;
const H = ['--header', `authorization: Bearer ${GH}`];
const data = JSON.parse(execFileSync('curl', ['-s', ...H, 'https://api.github.com/repos/lzy1102/openaide/actions/runs?per_page=3'], { maxBuffer: 10e6 }).toString());
const run = (data.workflow_runs as any[]).find((r) => r.head_branch === 'v0.3.2');
const jobs = JSON.parse(execFileSync('curl', ['-s', ...H, run.jobs_url], { maxBuffer: 20e6 }).toString());
for (const j of jobs.jobs as any[]) {
  console.log(`\n### ${j.name} -> ${j.conclusion}`);
  const log = execFileSync('curl', ['-sL', ...H, `https://api.github.com/repos/lzy1102/openaide/actions/jobs/${j.id}/logs`], { maxBuffer: 40e6 }).toString();
  const tail = log.split('\n').filter((l) => /error|ERR!|failed|403|401|E404|npm/i.test(l)).slice(-12);
  console.log(tail.join('\n'));
}
