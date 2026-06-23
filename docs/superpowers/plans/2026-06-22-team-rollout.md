# Team Rollout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 kbdiag 可以在几十台主机上团队使用：支持 `~/.kbdiagrc` 配置覆盖、sys_stat_statements 可操作安装提示、`kbdiag update` 自更新命令、`scripts/deploy.sh` 批量部署脚本。

**Architecture:** `~/.kbdiagrc` 在 `dist/kbdiag` 的最顶部 sourced（在 core.sh 变量声明之前），用 `${VAR:-default}` 模式自然被覆盖。`update` 命令封装成 `lib/cmd_update.sh` 独立文件。`sys_stat_statements` 提示提取成 `_stmt_install_hint()` 函数，在 `cmd_stmt` 和 `diagnose` 两处复用。`scripts/deploy.sh` 是独立脚本，不入 dist。

**Tech Stack:** Pure Bash, curl, scp/ssh

## Global Constraints

- 所有 `--help` 文本、`scripts/deploy.sh` 注释、`kbdiag update` 输出必须用英文
- `~/.kbdiagrc` 加载须静默（`2>/dev/null || true`），不破坏 `set -euo pipefail`
- `kbdiag update` 用 `readlink -f` 获取自身路径，兼容 symlink；失败时用 `$0`
- `scripts/deploy.sh` 不依赖 dist 以外的任何文件；目标路径默认 `~/kbdiag`
- 改完 `lib/*.sh` 必须 `bash build.sh` 重建 `dist/kbdiag`
- 测试命令：`bash test/run_tests.sh <suite>`，通过 SSH 在 Lima VM 执行

---

### Task 1: `~/.kbdiagrc` 支持

**Files:**
- Modify: `build.sh`（在写入 KBDIAG_VERSION 行后紧接着写入 source 语句）
- Modify: `dist/kbdiag`（build 产物，rebuild 自动更新）
- Modify: `README.md`（中英文各加 Configuration 节）
- Test: `test/cases/test_conf.sh`（追加 1 个测试）

**Interfaces:**
- Produces: `dist/kbdiag` 脚本顶部有 `[[ -f ~/.kbdiagrc ]] && source ~/.kbdiagrc` 一行，位于 `KBDIAG_VERSION=` 之后、core.sh 内容之前

- [ ] **Step 1: 追加测试到 `test/cases/test_conf.sh`**

```bash
test_kbdiagrc_overrides_port() {
  # 写一个临时 .kbdiagrc，设置一个不存在的端口，确认 status 连接失败而非走默认 54321
  # 实际上我们只验证 .kbdiagrc 被 source（用一个无害的变量覆盖）
  local out
  out=$(ssh_node1 "echo 'KB_QUERY_TIMEOUT=1' > ~/.kbdiagrc && $KBDIAG_REMOTE status; rm -f ~/.kbdiagrc" || true)
  # status 应该仍能运行（timeout=1 足够本地查询），不 crash
  assert_not_contains "$out" ": line "
  assert_not_contains "$out" "command not found"
}
```

- [ ] **Step 2: 确认测试当前不受影响（先跑确保基线）**

```bash
bash test/run_tests.sh conf
```

- [ ] **Step 3: 修改 `build.sh`，在 `KBDIAG_VERSION=` 行后加入 source**

当前 build.sh 第 13 行：
```bash
  echo "KBDIAG_VERSION=\"${KBDIAG_VERSION}\""
  echo ''
```

改为：
```bash
  echo "KBDIAG_VERSION=\"${KBDIAG_VERSION}\""
  echo '[[ -f ~/.kbdiagrc ]] && source ~/.kbdiagrc || true'
  echo ''
```

- [ ] **Step 4: 重建**

```bash
bash build.sh
```
预期：`Built: dist/kbdiag`

验证 dist/kbdiag 第 4 行是：
```
[[ -f ~/.kbdiagrc ]] && source ~/.kbdiagrc || true
```

```bash
sed -n '4p' dist/kbdiag
```

- [ ] **Step 5: 运行测试**

```bash
bash test/run_tests.sh conf
```
预期：包含 `test_kbdiagrc_overrides_port` PASS，无其他 FAIL。

- [ ] **Step 6: 更新 README（英文节）**

在 `## Environment Variables` 节前插入：

```markdown
## Configuration File

Per-host settings can be placed in `~/.kbdiagrc` (sourced at startup before any defaults):

```bash
# ~/.kbdiagrc — example
KB_PORT=5432
KB_BIN_DIR=/opt/kingbase/bin
KB_SUPERUSER=dba
KB_SLOW_THRESHOLD=3
```
```

- [ ] **Step 7: 更新 README（中文节）**

在 `## 环境变量` 节前插入：

```markdown
## 配置文件

每台主机的个性化配置放在 `~/.kbdiagrc`（启动时自动加载，优先于默认值）：

```bash
# ~/.kbdiagrc 示例
KB_PORT=5432
KB_BIN_DIR=/opt/kingbase/bin
KB_SUPERUSER=dba
KB_SLOW_THRESHOLD=3
```
```

- [ ] **Step 8: Commit**

```bash
git add build.sh dist/kbdiag README.md test/cases/test_conf.sh
git commit -m "feat: source ~/.kbdiagrc at startup for per-host config"
```

---

### Task 2: sys_stat_statements 可操作安装提示

**Files:**
- Modify: `lib/cmd_stmt.sh`（提取 `_stmt_install_hint()`，替换现有两处 warn）
- Modify: `lib/cmd_diagnose.sh`（`_diag_stmt_top` 中也调用同一 hint）
- Modify: `dist/kbdiag`（rebuild 产物）
- Test: `test/cases/test_stmt.sh`（追加 1 个测试）

**Interfaces:**
- Produces: `_stmt_install_hint()` 函数，无参数，输出到 stdout，供 `cmd_stmt` 和 `_diag_stmt_top` 调用

- [ ] **Step 1: 追加测试到 `test/cases/test_stmt.sh`**

```bash
test_stmt_no_ext_shows_hint() {
  # 如果扩展不可用，输出应包含 shared_preload_libraries
  # 如果扩展可用，此测试直接 skip
  local avail
  avail=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT count(*) FROM sys_catalog.sys_class WHERE relname='sys_stat_statements';\"" 2>/dev/null | tr -d '[:space:]')
  [[ "${avail:-0}" -gt 0 ]] && { _pass; return; }  # extension present, skip
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt" || true)
  assert_contains "$out" "shared_preload_libraries"
}
```

- [ ] **Step 2: 确认基线**

```bash
bash test/run_tests.sh stmt
```
记录当前 pass 数量。

- [ ] **Step 3: 在 `lib/cmd_stmt.sh` 中，将 `_stmt_check_ext` 后紧接着添加 `_stmt_install_hint()`**

在 `_stmt_check_ext()` 结束的 `}` 后面插入：

```bash
_stmt_install_hint() {
  local current_val
  current_val=$(ksql_q "SHOW shared_preload_libraries;" 2>/dev/null | tr -d '[:space:]')
  warn "sys_stat_statements extension is not loaded"
  printf "\n"
  printf "  Current:  shared_preload_libraries = '%s'\n" "${current_val:-<empty>}"
  printf "\n"
  printf "  To enable:\n"
  printf "    1. Edit \$KB_DATA_DIR/kingbase.conf\n"
  printf "       Add 'sys_stat_statements' to shared_preload_libraries\n"
  printf "    2. Restart KingbaseES\n"
  printf "    3. Optional tuning:\n"
  printf "         sys_stat_statements.max = 1000\n"
  printf "         sys_stat_statements.track = all\n"
  printf "\n"
  printf "  Check current value:  kbdiag params shared_preload_libraries\n"
}
```

- [ ] **Step 4: 替换 `cmd_stmt()` 中现有的 warn 调用**

当前代码（`lib/cmd_stmt.sh` 中的 `cmd_stmt()`）：
```bash
  if ! _stmt_check_ext; then
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      echo '{"command":"stmt","error":"sys_stat_statements not available","hint":"shared_preload_libraries = '"'"'sys_stat_statements'"'"' (需重启)"}'
    else
      warn "sys_stat_statements 扩展未启用"
      echo "  启用方式: shared_preload_libraries = 'sys_stat_statements'  (需重启)"
    fi
    return 0
  fi
```

替换为：
```bash
  if ! _stmt_check_ext; then
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      local cur_val
      cur_val=$(ksql_q "SHOW shared_preload_libraries;" 2>/dev/null | tr -d '[:space:]')
      echo "{\"command\":\"stmt\",\"error\":\"sys_stat_statements not loaded\",\"current_shared_preload_libraries\":\"${cur_val}\",\"hint\":\"add sys_stat_statements to shared_preload_libraries and restart\"}"
    else
      _stmt_install_hint
    fi
    return 0
  fi
```

- [ ] **Step 5: 重建并运行测试**

```bash
bash build.sh && bash test/run_tests.sh stmt
```
预期：所有之前通过的测试继续通过，`test_stmt_no_ext_shows_hint` PASS。

- [ ] **Step 6: Commit**

```bash
git add lib/cmd_stmt.sh dist/kbdiag test/cases/test_stmt.sh
git commit -m "feat: add actionable sys_stat_statements install hint"
```

---

### Task 3: `kbdiag update` 自更新命令

**Files:**
- Create: `lib/cmd_update.sh`
- Modify: `build.sh`（lib 列表 + USAGE + dispatch）
- Modify: `dist/kbdiag`（rebuild 产物）
- Modify: `README.md`（中英文各加 update 用法）
- Test: `test/cases/test_update.sh`（新建，2 个测试）

**Interfaces:**
- Produces: `cmd_update()` 函数，无参数，从 GitHub 下载最新 dist/kbdiag 覆盖自身

- [ ] **Step 1: 新建 `test/cases/test_update.sh`**

```bash
test_update_version_flag_works() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --version")
  assert_contains "$out" "kbdiag"
  assert_contains "$out" "v"
}

test_update_cmd_exists() {
  # 验证 update 子命令被识别（离线环境会 curl 失败，但不应报 "Unknown command"）
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE update" 2>&1 || true)
  assert_not_contains "$out" "Unknown command"
}
```

- [ ] **Step 2: 新建 `lib/cmd_update.sh`**

```bash
# cmd_update.sh — self-update from GitHub

cmd_update() {
  local url="https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag"
  local self
  self=$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")

  printf "Current : kbdiag %s\n" "$KBDIAG_VERSION"
  printf "Source  : %s\n" "$url"
  printf "Target  : %s\n\n" "$self"

  if ! command -v curl >/dev/null 2>&1; then
    echo "Error: curl not found" >&2
    exit 1
  fi

  local tmp
  tmp=$(mktemp)
  if curl -fsSL "$url" -o "$tmp" 2>&1; then
    local new_ver
    new_ver=$(grep 'KBDIAG_VERSION=' "$tmp" | head -1 | sed 's/.*="\(.*\)"/\1/')
    chmod +x "$tmp"
    mv "$tmp" "$self"
    printf "Updated : kbdiag %s\n" "$new_ver"
  else
    rm -f "$tmp"
    echo "Error: download failed" >&2
    exit 1
  fi
}
```

- [ ] **Step 3: 修改 `build.sh` — 加入 lib 列表、dispatch、USAGE**

在 `lib/cmd_license.sh` 后加入 `lib/cmd_update.sh`：
```bash
            lib/cmd_license.sh \
            lib/cmd_update.sh; do
```

在 dispatch 的 `license)` 行后加：
```bash
  update)    cmd_update ;;
```

在 USAGE 的 `[OPS]` 区块结尾加（在 `space` 行之后）：
```
  update              Update kbdiag to the latest version from GitHub
```

- [ ] **Step 4: 重建并运行测试**

```bash
bash build.sh && bash test/run_tests.sh update
```
预期：
```
[PASS] test_update_version_flag_works
[PASS] test_update_cmd_exists
```

- [ ] **Step 5: 更新 README（英文）**

在 `## Install` 节后加：

```markdown
## Update

```bash
~/kbdiag update
```
```

- [ ] **Step 6: 更新 README（中文）**

在 `## 安装` 节后加：

```markdown
## 更新

```bash
~/kbdiag update
```
```

- [ ] **Step 7: Commit**

```bash
git add lib/cmd_update.sh build.sh dist/kbdiag README.md test/cases/test_update.sh
git commit -m "feat: add kbdiag update self-update command"
```

---

### Task 4: `scripts/deploy.sh` 批量部署脚本

**Files:**
- Create: `scripts/deploy.sh`
- Create: `scripts/hosts.example`（示例主机列表）
- Modify: `README.md`（中英文各加 Team Deployment 节）

**Interfaces:**
- Consumes: `dist/kbdiag`（本地已 build 的产物）
- Produces: 每台目标主机上的 `~/kbdiag`，可立即执行

- [ ] **Step 1: 新建 `scripts/hosts.example`**

```
# kbdiag hosts file — one entry per line
# Format: user@host  (SSH will use default port, or set SSH_PORT env var)
# Lines starting with # are ignored

# kevin@db-node1.example.com
# kevin@db-node2.example.com
# kevin@192.168.1.10
```

- [ ] **Step 2: 新建 `scripts/deploy.sh`**

```bash
#!/usr/bin/env bash
# deploy.sh — push kbdiag to multiple DB hosts
# Usage: bash scripts/deploy.sh [hosts-file]
# Env:   SSH_PORT=22  DB_USER=kingbase  DEST=~/kbdiag
set -euo pipefail

HOSTS_FILE="${1:-}"
SSH_PORT="${SSH_PORT:-22}"
DB_USER="${DB_USER:-kingbase}"
DEST="${DEST:-~/kbdiag}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
KBDIAG="$SCRIPT_DIR/../dist/kbdiag"

if [[ ! -f "$KBDIAG" ]]; then
  echo "Error: dist/kbdiag not found. Run 'bash build.sh' first." >&2
  exit 1
fi

if [[ -z "$HOSTS_FILE" ]]; then
  echo "Usage: bash scripts/deploy.sh <hosts-file>" >&2
  echo "  hosts-file: one user@host per line (# = comment)" >&2
  echo "  Env vars:   SSH_PORT (default 22), DB_USER (default kingbase), DEST (default ~/kbdiag)" >&2
  echo "  Example:    scripts/hosts.example" >&2
  exit 1
fi

if [[ ! -f "$HOSTS_FILE" ]]; then
  echo "Error: hosts file not found: $HOSTS_FILE" >&2
  exit 1
fi

LOCAL_VER=$(grep 'KBDIAG_VERSION=' "$KBDIAG" | head -1 | sed 's/.*="\(.*\)"/\1/')
echo "Deploying kbdiag ${LOCAL_VER} → ${DEST}"
echo ""

ok=0; fail=0
while IFS= read -r entry || [[ -n "$entry" ]]; do
  [[ -z "$entry" || "$entry" == \#* ]] && continue
  printf "  %-40s" "$entry"
  if scp -q -P "$SSH_PORT" "$KBDIAG" "${entry}:${DEST}.tmp" 2>/dev/null \
     && ssh -p "$SSH_PORT" "$entry" "mv ${DEST}.tmp ${DEST} && chmod +x ${DEST} && ${DEST} --version" 2>/dev/null; then
    ok=$(( ok + 1 ))
  else
    printf "  FAIL\n"
    fail=$(( fail + 1 ))
  fi
done < "$HOSTS_FILE"

echo ""
printf "Done: %d succeeded, %d failed\n" "$ok" "$fail"
[[ $fail -eq 0 ]]
```

- [ ] **Step 3: 赋权**

```bash
chmod +x scripts/deploy.sh
```

- [ ] **Step 4: 手动验证脚本语法**

```bash
bash -n scripts/deploy.sh && echo "syntax OK"
```
预期：`syntax OK`

验证无主机文件时的错误输出：
```bash
bash scripts/deploy.sh 2>&1 | head -3
```
预期第一行：`Usage: bash scripts/deploy.sh <hosts-file>`

- [ ] **Step 5: 更新 README（英文）**

在 `## Update` 节后加：

```markdown
## Team Deployment

Push kbdiag to multiple hosts at once:

```bash
# Create a hosts file (one user@host per line)
cp scripts/hosts.example my-hosts.txt
vim my-hosts.txt

# Deploy
bash scripts/deploy.sh my-hosts.txt

# Custom SSH port or destination
SSH_PORT=2222 DB_USER=kingbase bash scripts/deploy.sh my-hosts.txt
```
```

- [ ] **Step 6: 更新 README（中文）**

在 `## 更新` 节后加：

```markdown
## 团队批量部署

一次推送到多台主机：

```bash
# 创建主机列表文件（每行 user@host）
cp scripts/hosts.example my-hosts.txt
vim my-hosts.txt

# 批量部署
bash scripts/deploy.sh my-hosts.txt

# 自定义 SSH 端口或目标路径
SSH_PORT=2222 bash scripts/deploy.sh my-hosts.txt
```
```

- [ ] **Step 7: Commit**

```bash
git add scripts/deploy.sh scripts/hosts.example README.md
git commit -m "feat: add scripts/deploy.sh for batch deployment to multiple hosts"
```

---

### Task 5: 全量测试 + push

- [ ] **Step 1: 全量测试**

```bash
bash test/run_tests.sh
```
预期：全部 PASS，零 FAIL。

- [ ] **Step 2: 修复失败（如有）**

- [ ] **Step 3: Push**

```bash
git push
```

---

## 自检

| 需求 | 覆盖 Task |
|------|-----------|
| `~/.kbdiagrc` 在 core.sh 变量声明前 source | Task 1 |
| source 失败不中断脚本（`\|\| true`） | Task 1 |
| README 中英文各有 Configuration 节 | Task 1 |
| sys_stat_statements 未启用时展示当前值 + 分步操作 | Task 2 |
| JSON 模式也返回结构化错误（含 current_shared_preload_libraries） | Task 2 |
| `kbdiag update` 自更新，打印旧/新版本号 | Task 3 |
| `kbdiag update` curl 不可用时给出明确报错 | Task 3 |
| README 中英文各有 Update 节 | Task 3 |
| `scripts/deploy.sh` 接受 hosts 文件，逐行 scp+ssh | Task 4 |
| 缺少 dist/kbdiag 时给出明确报错 | Task 4 |
| 支持 SSH_PORT / DB_USER / DEST 环境变量覆盖 | Task 4 |
| README 中英文各有 Team Deployment 节 | Task 4 |
| 全量测试通过 | Task 5 |
