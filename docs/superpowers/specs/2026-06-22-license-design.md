# License 授权检查功能设计

**日期**: 2026-06-22  
**状态**: 已批准

## 背景

KingbaseES 提供若干内置函数查询授权信息。运维人员需要快速确认：授权是否有效、是否正式授权、剩余天数。目前 kbdiag 无任何授权相关检查。

## 数据来源

KingbaseES 提供两个关键函数（均在 `pg_catalog` schema）：

| 函数 | 返回类型 | 含义 |
|------|----------|------|
| `get_license_validdays()` | integer | 授权剩余天数（-2=永久，正数=剩余天，≤0 且非-2=异常/过期）|
| `get_license_info()` | text | 完整授权信息文本（序列号、用户名称、版本模板等）|

## validdays 语义

| 返回值 | 含义 | 输出状态 |
|--------|------|----------|
| `-2` | 永久授权（正式） | OK |
| `> KB_WARN_LICENSE_DAYS`（默认 30）| 剩余天数充足 | OK |
| `<= KB_WARN_LICENSE_DAYS` 且 `> 0` | 即将到期 | WARN |
| `<= 0`（且非 -2）| 已过期或状态异常 | FAIL |

**正式授权判断**：`validdays == -2` 视为永久正式授权。正数 validdays 视为限期授权（正式或试用未区分，展示剩余天数）。

## 新增阈值

`core.sh` 追加：
```bash
KB_WARN_LICENSE_DAYS="${KB_WARN_LICENSE_DAYS:-30}"
```

## 新增命令：`kbdiag license`

**实现文件**：`lib/cmd_license.sh`

**函数**：`cmd_license()`

### 默认输出

调用 `get_license_validdays()` 和 `get_license_info()`，从 info 文本中用 `grep` + `sed` 提取以下字段：

- 序列号
- 产品名称
- 细分版本模板名
- 用户名称
- 项目名称
- 生产日期

示例（永久授权）：
```
=== License ===
[OK]    授权状态: 永久授权（正式）
[INFO]  序列号:   CDE76922-2BC8-11EC-AE8E-000C29CBE49F
[INFO]  产品名称: KingbaseES V8
[INFO]  版本模板: SALES-企业版 V8R6
[INFO]  用户名称: 中国证券监督委员会
[INFO]  项目名称: 安可替代工程基础软件购置项目
[INFO]  生产日期: 2021-10-13
```

示例（即将到期）：
```
[WARN]  授权状态: 剩余 15 天，即将到期
```

示例（已过期）：
```
[FAIL]  授权状态: 已过期（validdays=0）
```

### `--verbose` 模式

追加 `get_license_info()` 原文完整输出。

### JSON 格式

```json
{
  "license": {
    "status": "ok",
    "validdays": -2,
    "serial": "CDE76922-2BC8-11EC-AE8E-000C29CBE49F",
    "product": "KingbaseES V8",
    "edition": "SALES-企业版 V8R6",
    "user": "中国证券监督委员会",
    "project": "安可替代工程基础软件购置项目",
    "issued": "2021-10-13"
  }
}
```

## `kbdiag status` 集成

`cmd_status()` 末尾追加一行授权摘要，仅调用 `get_license_validdays()`（不解析 info 文本，保持轻量）：

```
[OK]    License: 永久授权（正式）
[WARN]  License: 剩余 15 天到期
[FAIL]  License: 已过期
```

## `kbdiag all` 集成

不单独调用 `cmd_license`。`all` 已包含 `cmd_status`，status 中的摘要行已覆盖。

## `kbdiag check` 集成

不纳入。`check` 专注运行时健康指标（连接数、延迟、事务等），license 是静态配置信息，性质不同。

## dispatch 入口

`kbdiag.sh` 及 `dist/kbdiag` dispatch 块新增：
```bash
license)     cmd_license ;;
```

help 文本 `[OPS]` 区块追加：
```
  license             License 授权状态（有效期、正式/试用）
```

## 文件变更清单

| 文件 | 变更 |
|------|------|
| `lib/core.sh` | 新增 `KB_WARN_LICENSE_DAYS` 阈值 |
| `lib/cmd_license.sh` | 新建，实现 `cmd_license()` |
| `lib/cmd_status.sh` | 末尾追加 license 摘要行 |
| `kbdiag.sh` | dispatch + help 文本 |
| `dist/kbdiag` | `build.sh` 重新打包生成 |
