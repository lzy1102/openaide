/** 真实浏览器冒烟：browser_search（Bing）+ browser_navigate + click/type 链路 */
import { browserPlugin } from './packages/tools/src/browser.js';

const get = (name: string) => browserPlugin.tools!.find((t) => t.name === name)!;

console.log('=== 1. browser_search: OpenAIDE github ===');
const s = await get('browser_search').handler({ query: 'OpenAIDE github agent', max_results: 5 }, 's1');
console.log(String(s.content).slice(0, 700));
if (s.error) console.log('ERROR:', s.error);

console.log('\n=== 2. browser_navigate: example.com ===');
const n = await get('browser_navigate').handler({ url: 'https://example.com' }, 's1');
console.log(String(n.content).slice(0, 400));
if (n.error) console.log('ERROR:', n.error);

console.log('\n=== 3. snapshot → type → submit 链路（example.com 无输入框，改用 bing）===');
const nav2 = await get('browser_navigate').handler({ url: 'https://www.bing.com' }, 's1');
// 从快照行解析出第一个 input 的 ref
const m = /r(\d+)\. input:/m.exec(nav2.content ?? '');
console.log('input ref:', m ? `r${m[1]}` : '(none)');
if (m) {
  const t = await get('browser_type').handler(
    { text: 'playwright mcp', ref: `r${m[1]}`, submit: true },
    's1',
  );
  console.log(String(t.content).slice(0, 300));
} else {
  console.log(nav2.content?.slice(0, 300));
}

await get('browser_close').handler({}, 's1');
console.log('\nDONE');
