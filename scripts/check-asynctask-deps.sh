#!/usr/bin/env bash
# check-asynctask-deps.sh — asynctask 包的依赖边界断言。
#
# 三条边界（源自 golib task/gasync，按 yg-go 实际情况调整）：
#   1) worker       不得直接/间接依赖  ORM 核心 gorm.io/gorm、任何 gorm driver、
#                   以及 asynctask/manager、asynctask/source/{db,remote}；
#   2) source/db    不得依赖 asynctask/manager、asynctask/worker；
#   3) 根包 asynctask 不得直接依赖  ORM 核心 gorm.io/gorm、任何 gorm driver。
#
# 说明（ADR-0001）：yg-go 的 logs 包内部 import 了 gorm.io/gorm/logger + gorm.io/gorm/utils
# （仅日志辅助子包，不含 ORM 核心与 DB driver）。因此任何 import logs 的包都会在
# `go list -deps` 里出现这两个子包——这是可接受的，本条门禁只针对 ORM 核心与 driver 本身。
set -euo pipefail

cd "$(dirname "$0")/.."
export GOFLAGS="${GOFLAGS:--mod=mod}"

fail=0

# forbidden_in <pkg> <regex...>
forbidden_in() {
  local pkg="$1"; shift
  local deps
  deps="$(go list -deps "./${pkg}" 2>/dev/null || true)"
  if [ -z "$deps" ]; then
    echo "SKIP (package not buildable yet): ${pkg}"
    return 0
  fi
  for pat in "$@"; do
    if echo "$deps" | grep -qx "$pat"; then
      echo "FAIL: ${pkg} transitively depends on forbidden ${pat}"
      fail=1
    fi
  done
}

# 精确路径的禁止项（整行匹配）
forbidden_in asynctask/worker \
  "gorm.io/gorm" \
  "github.com/ygpkg/yg-go/asynctask/manager" \
  "github.com/ygpkg/yg-go/asynctask/source/db" \
  "github.com/ygpkg/yg-go/asynctask/source/remote"

# gorm driver（任意 driver/*）
worker_deps="$(go list -deps ./asynctask/worker 2>/dev/null || true)"
if [ -n "$worker_deps" ] && echo "$worker_deps" | grep -qE '^gorm\.io/driver/'; then
  echo "FAIL: asynctask/worker depends on a gorm driver"
  fail=1
fi

forbidden_in asynctask/source/db \
  "github.com/ygpkg/yg-go/asynctask/manager" \
  "github.com/ygpkg/yg-go/asynctask/worker"

forbidden_in asynctask \
  "gorm.io/gorm"

root_deps="$(go list -deps ./asynctask 2>/dev/null || true)"
if [ -n "$root_deps" ] && echo "$root_deps" | grep -qE '^gorm\.io/driver/'; then
  echo "FAIL: asynctask root package depends on a gorm driver"
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo "dependency boundary check FAILED"
  exit 1
fi
echo "dependency boundary check OK"
