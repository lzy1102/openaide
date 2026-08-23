# OpenAIDE —— 统一任务入口（npm scripts 的薄封装，真源在 package.json）
# Windows 用户若无 make，直接用等价 npm 命令即可（见 USAGE.md 开发表格）。

.PHONY: help install dev repl serve build typecheck test test-pkg clean publish

help:
	@echo "OpenAIDE tasks:"
	@echo "  make install    安装依赖 + 全局 link openaide 命令"
	@echo "  make dev        开发运行（免编译，默认 TUI/REPL）"
	@echo "  make repl       同上（显式子命令）"
	@echo "  make serve      启动 API + WebUI (:8080)"
	@echo "  make build      编译所有包到 dist/（tsc + vite）"
	@echo "  make typecheck  全部包类型检查"
	@echo "  make test       全部测试（node --test + vitest）"
	@echo "  make test-pkg P=core   只跑某个包的测试"
	@echo "  make clean      清理所有 dist 产物"
	@echo "  make publish    按依赖顺序发布 @openaide/* 包"

install:
	node scripts/install.mjs

dev:
	npm run dev

repl:
	npm run dev:repl

serve:
	npm run dev:serve

build:
	npm run build

typecheck:
	npm run typecheck

test:
	npm test

test-pkg:
	node scripts/test.mjs $(P)

clean:
	node -e "const fs=require('fs'),p=require('path');const rm=d=>fs.rmSync(d,{recursive:true,force:true});rm('frontend/dist');for(const e of fs.readdirSync('packages',{withFileTypes:true})){if(e.isDirectory())rm(p.join('packages',e.name,'dist'))}console.log('dist cleaned')"

publish:
	node scripts/publish.mjs
