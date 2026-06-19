# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

kbdiag — KingbaseES 命令行诊断工具，Shell 编写，不进交互界面直接查实例状态。支持单机和主备集群（repmgr）。

## Development Environment

### Test VMs (Lima)

| Node | Role | SSH | Internal IP |
|------|------|-----|-------------|
| kes-node1 | primary | `ssh -p 57103 kevin@127.0.0.1` | 192.168.105.10 |
| kes-node2 | standby | `ssh -p 57123 kevin@127.0.0.1` | 192.168.105.11 |

### KingbaseES

- Port: 54321
- OS user: `kingbase`
- Connect: `ksql test system` (local socket, no password)
- Cluster manager: repmgr
- Replication user: esrep

### Login sequence

```bash
ssh -p 57103 kevin@127.0.0.1   # node1 (primary)
# or
ssh -p 57123 kevin@127.0.0.1   # node2 (standby)

sudo -i
su - kingbase
ksql test system                # connect to DB
```

### Check cluster status

```bash
su - kingbase
repmgr cluster show
```

## Architecture

Pure Shell scripts — no build step, no dependencies. Scripts run directly on the DB host as the `kingbase` OS user.

Design rules:
- **Non-interactive**: never open `psql`/`ksql` interactive sessions; use `ksql -c "..."` or `ksql -f file.sql`
- **Single-node + HA**: every check must degrade gracefully when repmgr is absent (standalone mode)
- **No sudo assumed**: scripts run as `kingbase`; escalation must be explicit and documented

## Git / Deploy

- Remote: `https://github.com/Kevin-wenyu/kbdiag.git`
- Branch strategy: push directly to `main`
