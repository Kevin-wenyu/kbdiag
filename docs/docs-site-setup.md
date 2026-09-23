# 初始化 kbdiag 文档网站

脚本：`scripts/init-docs.py`。生成独立的 OINK 网站，默认位于 kbdiag 同级的 `kbdiag-docs`，不修改工具代码、不自动创建 GitHub 仓库、不推送或上线。

## 依赖

- 生成：Python 3.9+、Git，以及访问 GitHub 的网络连接。无需 pip 包。
- 预览和构建：Hugo **Extended 0.165.0**、Go **1.27.0+**。首次构建需下载 OINK 模块。
- 发布命令：GitHub CLI，先执行 `gh auth login`。

Hugo 官方版本下载：<https://github.com/gohugoio/hugo/releases/tag/v0.165.0>。macOS 对应资源是 `hugo_extended_0.165.0_darwin-universal.pkg`；Homebrew 当前版本不一定与固定版本相同。

## 执行

在 kbdiag 仓库根目录：

```bash
python3 scripts/init-docs.py
```

自定义目标目录，并生成后立即严格构建：

```bash
python3 scripts/init-docs.py --output ../kbdiag-docs --check
```

目标目录必须不存在，脚本不会覆盖已有网站。准备过程失败会清理临时文件；使用 `--check` 时，构建通过才会保留最终目录。仅检查已生成的网站：

```bash
python3 scripts/init-docs.py --output ../kbdiag-docs --check-only
```

预览：

```bash
cd ../kbdiag-docs
hugo server
```

默认首页为英文，中文入口为 `/zh/`。脚本生成首页、快速开始和 README 使用手册快照，后续可以按场景细分页面。它保留 OINK 许可证，固定模板提交和主题版本，并创建独立 Git 仓库及本地初始提交。网站手册不会自动跟随工具 README 更新。

## 发布

脚本结束会打印与你的 `--owner`、`--repo` 对应的命令。以下适用于 **尚不存在** 的 `Kevin-wenyu/kbdiag-docs` 仓库：

```bash
cd ../kbdiag-docs
gh repo create Kevin-wenyu/kbdiag-docs --public --source . --remote origin
gh api --method POST repos/Kevin-wenyu/kbdiag-docs/pages -f build_type=workflow
git push -u origin main
```

先完成严格构建再发布。任何一步报错应停止并解决，不要继续后续命令。如果仓库已存在，请设置对应 origin、检查远端历史，并在 Settings → Pages 中选择 GitHub Actions，不要强推覆盖。

推送后检查 GitHub Actions 的部署结果，访问其输出的网站地址，验证搜索、中英文切换及文档链接。源码按钮指向 `Kevin-wenyu/kbdiag`，编辑文档链接指向文档仓库。

## 核实依据

- <https://oink.pgsty.com/docs/start/starter/>
- <https://oink.pgsty.com/docs/admin/deploy/>
- 固定模板：<https://github.com/pgsty/oink-starter/tree/137843b25bacd76ddd1f7ce71330bf2e3155b954>
