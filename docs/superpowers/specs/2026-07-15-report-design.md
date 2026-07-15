# `report` — 一键巡检报告设计（R2）

需求来源：2026-07-14-next-features-requirements.md R2。定位：看层聚合输出，纯编排，不采新数据。

## 接口

```
kbdiag report [outfile]     # 默认 kbdiag-report-<host>-<ts>.md，写当前目录
```

- exit 0/1/2 = 报告总评 OK/WARN/FAIL（可直接进 cron 判断）
- `--format json`：stdout 输出各 section 状态汇总 + 报告路径（报告文件照常生成）
- 单机降级：cluster/replication section 自动跟随底层命令的 standalone 降级

## 结构（Markdown）

1. 头部：主机、实例版本、角色、生成时间、kbdiag 版本
2. **Executive summary**：总评（OK/WARN/FAIL）+ 三色计数 + 全部 WARN/FAIL 条目汇总表（含所属 section）——结论先行，甲方只看这节
3. 明细 section（每节 = 对应命令的文本输出装进 ```text 块）：
   status / license / check / cluster ready / replication / space / backup / stat / idx / audit
4. **Recommendations**：advisor 输出
5. 尾注：阈值环境变量说明 + 交叉验证命令提示

## 实现要点

- **复用方式**：直接调用 cmd_* 函数并捕获输出（`out=$(cmd_x 2>&1) || rc=$?`），不重写任何查询
- **颜色**：进入 report 即 `NO_COLOR=1; _init_colors`，输出天然纯文本，无需 ANSI 剥离
- **汇总统计**：对捕获文本 `grep -c '^\[OK\]'` 等计数标记行；WARN/FAIL 行提取进汇总表（`|` 替换为 `/` 防表格断裂）
- **总评**：任一 FAIL → FAIL(2)；否则任一 WARN → WARN(1)；否则 OK(0)
- **进度**：文本模式下每节完成打一行进度到终端（JSON 模式静默）
- 耗时目标 < 90s（各命令 perf 预算之和的下界；stat 含 5s 采样窗口）

## 与 all / diagnose 的边界（对抗审查结论落实）

- `all`：命令输出顺序堆叠，给自己看，无汇总无文件
- `diagnose`：故障时的根因链
- `report`：例行巡检交付物——结论先行、分级汇总、单文件可存档；judgment 全部来自底层命令既有判定，本命令零新增判断逻辑

## 测试

test_report.sh：生成成功且文件非空、含 Executive summary、含全部 section 标题、exit ∈ {0,1}、
WARN/FAIL 计数与汇总表行数一致、--format json 合法、standby 节点可运行、自定义输出路径生效。
