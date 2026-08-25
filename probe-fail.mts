import { execFileSync } from 'node:child_process';
const GH = process.argv[2]!;
const RUN = process.argv[3]!;
const H = ['--header', `authorization: Bearer ${GH}`];
const out = execFileSync('curl', ['-s', ...H, `https://api.github.com/repos/lzy1102/openaide/actions/runs/${RUN}/jobs`], { maxBuffer: 20e6 }).toString();
const data = JSON.parse(out);
for (const j of data.jobs) {
  console.log(`\n### ${j.name} [${j.conclusion}] id=${j.id}`);
  for (const st of j.steps ?? []) {
    if (st.conclusion && st.conclusion !== 'success' && st.conclusion !== 'skipped')
      console.log(`   ✗ step: ${st.name} (${st.conclusion})`);
  }
}
// 拉第一个失败 job 的日志尾部
const failed = (data.jobs as any[]).find((j) => j.conclusion === 'failure');
if (failed) {
  const log = execFileSync('curl', ['-sL', ...H, `https://api.github.com/repos/lzy1102/openaide/actions/jobs/${failed.id}/logs`], { maxBuffer: 40e6 }).toString();
  const lines = log.split('\n');
  const hits = lines.filter((l) => /error|ERR!|✖|failed|not ok/i.test(l)).slice(-20);
  console.log('\n--- failure log tail ---\n' + hits.join('\n').slice(-3000));
}
