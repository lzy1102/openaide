/**
 * 配置加载/落盘测试:
 *  - 首次运行自动生成模板：不含明文 api_key（env 覆盖不落盘）、不含机器绝对路径
 *  - 自定义路径（测试传参）不触发自动创建
 *  - saveConfig → loadConfig 往返保持 compress 字段
 */
import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { defaultConfigPath, loadConfig, saveConfig } from '@openaide/config';

describe('loadConfig auto-generate', () => {
  test('first run writes a sanitized template to the default path', () => {
    const dir = mkdtempSync(join(tmpdir(), 'openaide-cfg-'));
    const prevDataDir = process.env.OPENAIDE_DATA_DIR;
    const prevKey = process.env.OPENAIDE_API_KEY;
    process.env.OPENAIDE_DATA_DIR = dir;
    process.env.OPENAIDE_API_KEY = 'sk-secret-do-not-persist';
    try {
      const cfgPath = defaultConfigPath();
      assert.ok(!existsSync(cfgPath), 'precondition: no config file yet');

      loadConfig(); // 触发自动落盘

      assert.ok(existsSync(cfgPath), 'template should be created');
      const text = readFileSync(cfgPath, 'utf8');
      assert.ok(!text.includes('sk-secret-do-not-persist'), 'env api key must not be persisted');
      assert.ok(!text.includes('data_dir'), 'machine-specific dirs must be omitted');
      assert.ok(!text.includes('plugins_dir'), 'machine-specific dirs must be omitted');
      assert.ok(text.includes('model:'), 'defaults documented in template');
      assert.ok(text.includes('compress:'), 'compress section present');

      // 运行时 env 覆盖仍然生效
      const cfg = loadConfig();
      assert.equal(cfg.llm.apiKey, 'sk-secret-do-not-persist');
    } finally {
      if (prevDataDir === undefined) delete process.env.OPENAIDE_DATA_DIR;
      else process.env.OPENAIDE_DATA_DIR = prevDataDir;
      if (prevKey === undefined) delete process.env.OPENAIDE_API_KEY;
      else process.env.OPENAIDE_API_KEY = prevKey;
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test('custom config path is never auto-created (no test side effects)', () => {
    const dir = mkdtempSync(join(tmpdir(), 'openaide-cfg-'));
    try {
      const custom = join(dir, 'custom.yaml');
      const cfg = loadConfig(custom);
      assert.ok(!existsSync(custom), 'custom path must not be written');
      assert.equal(typeof cfg.kernel.maxRounds, 'number');
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe('config roundtrip', () => {
  test('saveConfig → loadConfig preserves compress options', () => {
    const dir = mkdtempSync(join(tmpdir(), 'openaide-cfg-'));
    try {
      const path = join(dir, 'config.yaml');
      const cfg = loadConfig(path);
      saveConfig({ ...cfg, kernel: { ...cfg.kernel, compress: { keepRecent: 4, maxChars: 800, summaryTokens: 300 } } }, path);

      const loaded = loadConfig(path);
      assert.deepEqual(loaded.kernel.compress, { keepRecent: 4, maxChars: 800, summaryTokens: 300 });
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
