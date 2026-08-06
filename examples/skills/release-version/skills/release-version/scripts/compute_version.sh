#!/usr/bin/env bash
# 计算下一个语义化版本号。
# 用法: compute_version.sh <major|minor|patch> [当前版本号，缺省时自动从 git tag 读取]
set -euo pipefail

TYPE="${1:?usage: compute_version.sh <major|minor|patch> [current-version]}"
CURRENT="${2:-$(git describe --tags --abbrev=0 2>/dev/null || echo '0.0.0')}"
CURRENT="${CURRENT#v}"  # 去掉 v 前缀，如 v1.2.3 -> 1.2.3

IFS='.' read -r MAJOR MINOR PATCH _ <<<"${CURRENT}"
MAJOR=${MAJOR:-0}; MINOR=${MINOR:-0}; PATCH=${PATCH:-0}

case "$TYPE" in
  major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
  minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
  patch) PATCH=$((PATCH + 1)) ;;
  *) echo "unknown type: $TYPE (expect major|minor|patch)" >&2; exit 1 ;;
esac

echo "${MAJOR}.${MINOR}.${PATCH}"
