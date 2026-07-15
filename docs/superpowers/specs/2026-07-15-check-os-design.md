# `check --os` — OS 配置符合性检查设计（R3）

需求来源：2026-07-14-next-features-requirements.md R3。范围钉死：只查**对数据库有直接、可解释影响**的 OS 配置，不做通用 OS 性能分析。

## 接口

```
kbdiag check          # 不变：15 项 DB 检查
kbdiag check --os     # 15 项 DB 检查 + OS 检查段，exit 聚合
```

- no-sudo 红线：读不到的项输出 SKIP（info 行 + json status="skip"），绝不报错
- `report` 的 Health check 段升级为 `cmd_check --os`（巡检交付物天然应含 OS 层）

## VM 实测（2026-07-15，AlmaLinux 8.10）

数据源全部免 root 可读；本环境天然命中 3 WARN + 1 SKIP，测试无需造数据：
THP=[always]、swappiness=20、NTPSynchronized=no、无 cpufreq（VM）。

## 检查项（8 项，每项都能回答"这项错了数据库会怎样"）

| 项 | 数据源 | 判定 | 对 DB 的影响 |
|----|--------|------|--------------|
| THP | /sys/kernel/mm/transparent_hugepage/enabled（含 redhat_ 旧路径回退）| [always] → WARN | 内存整理引发延迟尖刺 |
| swappiness | /proc/sys/vm/swappiness | > KB_WARN_SWAPPINESS(10) → WARN | 数据库缓存被换出 |
| swap 使用 | /proc/meminfo | 无 swap → OK；已用 >20% → WARN | 换页 = 内存压力信号 |
| overcommit | /proc/sys/vm/overcommit_memory | ≠2 → WARN | OOM killer 可能击杀数据库进程 |
| ulimit nofile | ulimit -n（kingbase 用户即 DB 进程真实限制）| < KB_WARN_NOFILE(65536) → WARN | 连接/文件句柄耗尽 |
| ulimit nproc | ulimit -u | < KB_WARN_NPROC(4096) → WARN | 无法 fork 后端进程 |
| 时钟同步 | timedatectl NTPSynchronized，回退 chronyc stratum | 未同步 → WARN；两者都没有 → SKIP | 复制延迟计算、PITR 时间戳失真 |
| 数据目录 FS | findmnt -T $data_dir | nfs/cifs → WARN；其余 OK 并展示挂载参数 | 网络 FS 的 fsync 语义风险 |
| CPU governor | cpu0 scaling_governor | powersave → WARN；无 cpufreq → SKIP（VM 常态）| 降频拉高查询延迟 |

## 实现

- `_check_os()` 放 lib/cmd_check.sh（属 check 的组成部分，不新增文件/顶层命令）
- cmd_check 接受 `--os`/`os` 子参数；dispatch 改为 `cmd_check "$SUBCMD" || exit $?`
- 聚合：`_check_os` 返回自身 0/1/2，并入 cmd_check 的 `_exit`
- SKIP 的 JSON 表达：`json_item "os_x" "skip" ...`（status 枚举新增 skip，消费方按非 warn/fail 处理）

## 测试

test_check.sh 追加：--os 后含 "OS check" 头与 THP 行、exit ∈ {0,1,2}、JSON 合法且含 os_* items、
不带 --os 时无 OS 输出（默认行为不变）。
