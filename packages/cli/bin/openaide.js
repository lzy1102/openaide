#!/usr/bin/env node
/**
 * openaide 命令行入口 —— 全局可执行加载器。
 * import 'tsx' 注册 TS/ESM 加载器，随后加载 TypeScript 入口（monorepo 各包 main 均指向 src/*.ts）。
 * 用途：npm link 后全局的 `openaide` 命令可直接使用，无需提前 npm run build。
 */
import 'tsx';
await import('../src/main.ts');