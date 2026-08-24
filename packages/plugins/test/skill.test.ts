/**
 * SKILL.md 生态兼容验收 —— dsh / agent-skills 可移植技能格式：
 * frontmatter 解析、零配置目录投喂、SYSTEM.md 优先级、与 manifest 组合。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { discover, loadPlugin, readDeclarativePersona, readSkillFile } from '../src/loader.js';

function makeDir(name = 'skill-case'): string {
  const root = mkdtempSync(join(tmpdir(), 'openaide-skill-'));
  const dir = join(root, name);
  mkdirSync(dir, { recursive: true });
  return dir;
}

test('readSkillFile：frontmatter 的 name/description 生效，正文为提示词', () => {
  const dir = makeDir('pdf-helper');
  writeFileSync(
    join(dir, 'SKILL.md'),
    [
      '---',
      'name: pdf-pro',
      'description: 处理 PDF 文件的技能',
      '---',
      '',
      '# PDF 处理流程',
      '1. 先抽取文本',
    ].join('\n'),
  );
  const skill = readSkillFile(dir);
  assert.equal(skill?.name, 'pdf-pro');
  assert.equal(skill?.description, '处理 PDF 文件的技能');
  assert.match(skill?.systemPrompt ?? '', /# PDF 处理流程/);
});

test('无 frontmatter：人格名回退目录名，全文即提示词', async () => {
  const dir = makeDir('code-review');
  writeFileSync(join(dir, 'SKILL.md'), 'You are a code reviewer.');
  // 零配置目录投喂：discover 直接命中、loadPlugin 生成虚拟插件
  assert.ok(discover(dirnameOf(dir)).includes(dir));
  const { plugin } = await loadPlugin(dir);
  assert.equal(plugin.name, 'code-review');
  assert.equal(plugin.persona && typeof plugin.persona === 'object' ? plugin.persona.name : '', 'code-review');
  assert.equal(readDeclarativePersona(dir)?.systemPrompt, 'You are a code reviewer.');
});

test('SYSTEM.md 优先于 SKILL.md（原生格式优先）', () => {
  const dir = makeDir();
  writeFileSync(join(dir, 'SYSTEM.md'), 'native wins');
  writeFileSync(join(dir, 'SKILL.md'), 'skill loses');
  assert.equal(readDeclarativePersona(dir)?.systemPrompt, 'native wins');
});

test('openaide.yaml + SKILL.md：manifest.persona 覆盖技能名', async () => {
  const dir = makeDir('my-pack');
  writeFileSync(join(dir, 'openaide.yaml'), 'name: my-pack\npersona: voyager\n');
  writeFileSync(join(dir, 'SKILL.md'), '---\nname: ignored\n---\nbody here');
  const { plugin } = await loadPlugin(dir);
  const persona = plugin.persona as { name: string; systemPrompt: string };
  assert.equal(persona.name, 'voyager', 'manifest 声明的人格名优先');
  assert.match(persona.systemPrompt, /body here/);
});

// 从任意目录向上取一层（tmp 根），供 discover 断言用
function dirnameOf(p: string): string {
  return p.split(/[\\/]/).slice(0, -1).join('/');
}
