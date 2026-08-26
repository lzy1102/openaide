import { browserPlugin } from './packages/tools/src/browser.js';
const get = (n: string) => browserPlugin.tools!.find((t) => t.name === n)!;
const gamePath = 'D:/project/android/openaide/eval-workspace/watermelon-game/index.html';

const nav = await get('browser_navigate').handler(
  { url: 'file:///' + gamePath.replace(/\\/g, '/') },
  's1',
);
console.log('=== 加载结果 ===');
console.log((nav.content ?? nav.error ?? '').slice(0, 700));

const click = await get('browser_click').handler({ ref: 'canvas' }, 's1').catch((e) => ({ error: e.message }));
console.log('\n=== 点击投放 ===');
console.log(String(click.content ?? click.error ?? '').slice(0, 250));

await new Promise((r) => setTimeout(r, 1200));
const snap = await get('browser_snapshot').handler({}, 's1');
console.log('\n=== 投放后页面状态 ===');
console.log(String(snap.content ?? snap.error ?? '').slice(0, 400));

await get('browser_close').handler({}, 's1');
console.log('\nVERIFY DONE');
