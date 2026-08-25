import { execFileSync } from 'node:child_process';
const GH = process.argv[2]!;
const RUN = process.argv[3]!;
const H = ['--header', `authorization: Bearer ${GH}`];
const data = JSON.parse(execFileSync('curl', ['-s', ...H, `https://api.github.com/repos/lzy1102/openaide/actions/runs/${RUN}/jobs`], { maxBuffer: 20e6 }).toString());
const failed = (data.jobs as any[]).find((j) => j.conclusion === 'failure');
console.log('failed job:', failed?.name, failed?.id);
const log = execFileSync('curl', ['-sL', ...H, `https://api.github.com/repos/lzy1102/openaide/actions/jobs/${failed.id}/logs`], { maxBuffer: 40e6 }).toString();
const lines = log.split('\n');
// 抓 not ok 与其后续 error 行
for (let i = 0; i < lines.length; i++) {
  if (/^not ok|error:|AssertionError/i.test(lines[i] ?? '')) {
    console.log(lines.slice(i, i + 6).join('\n'));
    console.log('---');
    i += 6;
  }
}
