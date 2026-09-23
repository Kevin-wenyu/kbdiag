#!/usr/bin/env python3
"""Create a standalone kbdiag OINK site. Python 3.9+, Git; no pip packages."""

import argparse
import io
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile


TEMPLATE = "https://github.com/pgsty/oink-starter.git"
REVISION = "137843b25bacd76ddd1f7ce71330bf2e3155b954"
PROJECT = Path(__file__).resolve().parents[1]


def run(*args, cwd=None, capture=False, env=None):
    return subprocess.run(args, cwd=cwd, check=True,
                          env=env,
                          stdout=subprocess.PIPE if capture else None)


def write(root, name, text):
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def page(root, name, title, body, weight=10, section=False):
    metadata = f"---\ntitle: {json.dumps(title, ensure_ascii=False)}\nweight: {weight}\n"
    if section:
        metadata += "type: docs\ncascade:\n  type: docs\n"
    write(root, name, metadata + "---\n\n" + body + "\n")


def check(root):
    for tool in ("hugo", "go"):
        if not shutil.which(tool):
            raise RuntimeError(f"缺少 {tool}。请安装 Hugo Extended 0.165.0 和 Go >= 1.27.0 后重试。")
    version = run("hugo", "version", capture=True).stdout.decode()
    if "extended" not in version.lower():
        raise RuntimeError(f"需要 Hugo Extended，当前为：{version.strip()}")
    if not re.search(r"v0\.165\.0(?:\D|$)", version):
        raise RuntimeError("请使用与发布工作流一致的 Hugo Extended 0.165.0。")
    print(version.strip(), flush=True)
    run("go", "version")
    env = dict(os.environ, GOWORK="off", HUGO_MODULE_WORKSPACE="off",
               HUGO_CACHEDIR=str(root / ".hugo_cache"))
    run("hugo", "--cleanDestinationDir", "--gc", "--minify",
        "--environment", "production", "--printPathWarnings", "--panicOnWarning", cwd=root, env=env)


def instructions(root, owner, repo):
    directory = shlex.quote(str(root))
    print(f"""
站点目录：{root}
默认英文首页，中文位于 /zh/。

本地预览：
  cd {directory}
  hugo server

发布前检查：
  python3 {shlex.quote(str(Path(__file__).resolve()))} --output {directory} --check-only

首次发布（需要 GitHub CLI 已登录，并且 {owner}/{repo} 尚不存在）：
  cd {directory}
  gh repo create {owner}/{repo} --public --source . --remote origin
  gh api --method POST repos/{owner}/{repo}/pages -f build_type=workflow
  git push -u origin main

如果仓库已经存在，请配置它的 origin，并在 Settings → Pages 选择 GitHub Actions，
确认远端历史后再推送。不要强制覆盖远端。
脚本已创建本地初始提交；后续修改需要自行 git add / git commit。
部署后在 Actions 中确认结果，预计地址：https://{owner}.github.io/{repo}/
""")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=PROJECT.parent / "kbdiag-docs",
                        help="目标目录，必须不存在（默认：kbdiag 同级的 kbdiag-docs）")
    parser.add_argument("--owner", default="Kevin-wenyu", help="GitHub 用户或组织")
    parser.add_argument("--repo", default="kbdiag-docs", help="新文档仓库名称")
    parser.add_argument("--check", action="store_true", help="生成后执行严格构建")
    parser.add_argument("--check-only", action="store_true", help="仅构建已生成的网站，不修改内容")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9-]*", args.owner):
        parser.error("owner 必须是 GitHub 用户或组织名称")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]*", args.repo):
        parser.error("repo 必须是 GitHub 仓库名称")
    if args.repo.lower() == f"{args.owner.lower()}.github.io":
        parser.error("本脚本生成项目站点，请使用 kbdiag-docs 等仓库名")
    root = args.output.expanduser().absolute()
    if args.check_only:
        if not (root / ".kbdiag-docs.json").is_file():
            parser.error("目标不是本脚本生成的网站")
        check(root)
        return
    if root.exists() or root.is_symlink():
        parser.error(f"目标已存在，未修改任何文件：{root}。检查已有站点请用 --check-only。")
    if not shutil.which("git"):
        parser.error("请先安装 Git")
    readme = (PROJECT / "README.md").read_text(encoding="utf-8")
    marker = '<a name="中文"></a>'
    if marker not in readme or '<a name="english"></a>' not in readme:
        raise RuntimeError("README 语言分段发生变化，请先更新脚本。")
    english, chinese = readme.split(marker, 1)
    english = english.split('<a name="english"></a>', 1)[1].strip()
    english = re.sub(r"\n---\s*$", "", english)
    chinese = chinese.strip()
    source = "https://github.com/Kevin-wenyu/kbdiag"
    base = f"https://{args.owner}.github.io/{args.repo}/"

    root.parent.mkdir(parents=True, exist_ok=True)
    # Build in an adjacent temporary directory; failure never damages the destination.
    with tempfile.TemporaryDirectory(prefix=".kbdiag-docs-", dir=root.parent) as tmp:
        tmp = Path(tmp)
        clone, site = tmp / "template", tmp / "site"
        print("下载并校验固定版本的 OINK Starter…", flush=True)
        run("git", "clone", "--quiet", TEMPLATE, str(clone))
        run("git", "-C", str(clone), "checkout", "--quiet", "--detach", REVISION)
        archive = run("git", "-C", str(clone), "archive", REVISION, capture=True).stdout
        site.mkdir()
        with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
            for member in tar.getmembers():
                path = Path(member.name)
                if path.is_absolute() or ".." in path.parts or not (member.isfile() or member.isdir()):
                    raise RuntimeError(f"模板包含不支持的归档条目：{member.name}")
            tar.extractall(site)

        config = (site / "examples/hugo.bilingual.yaml").read_text(encoding="utf-8")
        config = config.replace("title: &siteTitle Project Name", "title: &siteTitle kbdiag")
        config = config.replace("https://example.org/", base)
        config = config.replace("authors: Project contributors", "authors: kbdiag contributors")
        config = config.replace("params:\n  offline_search:",
                                f"params:\n  github_repo: https://github.com/{args.owner}/{args.repo}\n"
                                "  github_branch: main\n  offline_search:")
        write(site, "hugo.yaml", config)
        # Only freshly exported template samples are removed. Licenses remain intact.
        shutil.rmtree(site / "content")
        shutil.rmtree(site / "data/home")
        page(site, "content/_index.md", "kbdiag", "KingbaseES command-line DBA toolkit.")
        page(site, "content/_index.zh.md", "kbdiag", "KingbaseES 命令行诊断与 DBA 工具集。")
        for lang, manual in (("en", english), ("zh", chinese)):
            zh = lang == "zh"
            suffix = ".zh.md" if zh else ".md"
            docs = "文档" if zh else "Documentation"
            start = "快速开始" if zh else "Get started"
            reference = "使用手册" if zh else "User manual"
            page(site, "content/docs/_index" + suffix, docs,
                 "kbdiag — KingbaseES DBA toolkit.", section=True)
            path = site / ("content/docs/_index" + suffix)
            value = path.read_text(encoding="utf-8").replace(
                "weight: 10\n", "weight: 10\nmenus:\n  main:\n    identifier: docs\n    weight: 20\n", 1)
            path.write_text(value, encoding="utf-8")
            page(site, "content/docs/get-started/_index" + suffix, start,
                 ("使用 kingbase 系统用户，在 KingbaseES V8R6+ 主机执行。" if zh else
                  "Run as the kingbase OS user on a KingbaseES V8R6+ host.") +
                 '\n\n```bash\nsudo -i -u kingbase\ncurl -fsSL '
                 'https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag '
                 '-o ~/kbdiag\nchmod +x ~/kbdiag\n~/kbdiag status\n~/kbdiag check\necho $?\n```\n\n' +
                 ("`check`：0 正常，1 告警，2 故障。" if zh else
                  "`check`: 0 OK, 1 WARN, 2 FAIL."), section=True)
            page(site, "content/docs/reference/_index" + suffix, reference,
                 (f"内容来自 [kbdiag README]({source}#readme)，是生成时的快照。\n\n" if zh else
                  f"Snapshot of the [kbdiag README]({source}#readme) at generation time.\n\n") + manual,
                 weight=30, section=True)
            # JSON is valid YAML; no external YAML package is required.
            home = {
                "sections": ["hero", "cards"],
                "hero": {"align": "center", "eyebrow": "KingbaseES DBA Toolkit",
                         "title_lines": [{"words": [{"text": "kbdiag"}]}],
                         "lead": "巡检、深查、根因诊断。" if zh else "Inspect health, investigate performance, diagnose causes.",
                         "actions": [{"label": start, "url": "docs/get-started/", "style": "primary"},
                                     {"label": "GitHub", "url": source, "style": "ghost"}]},
                "cards": {"title": docs, "columns": 2, "items": [
                    {"title": start, "desc": "安装与首次检查" if zh else "Install and run your first check",
                     "url": "docs/get-started/"},
                    {"title": reference, "desc": "命令、参数与配置" if zh else "Commands, flags, and configuration",
                     "url": "docs/reference/"}]}}
            write(site, f"data/home/{lang}.yaml", json.dumps(home, ensure_ascii=False, indent=2) + "\n")
        write(site, "README.md", f"# kbdiag documentation\n\nWebsite: {base}\n\n"
              f"Tool source: {source}\n\nOINK Starter: {TEMPLATE} @ {REVISION}\n\n"
              "Requires Hugo Extended 0.165.0 and Go >= 1.27.0.\n\n"
              "Preview: `hugo server`\n\n"
              "The English and Chinese manuals are snapshots of the tool README; maintain them as the tool changes.\n")
        write(site, ".kbdiag-docs.json", json.dumps({"template": TEMPLATE, "revision": REVISION,
              "owner": args.owner, "repo": args.repo}, indent=2) + "\n")
        run("git", "init", "--quiet", "--initial-branch=main", str(site))
        # Record source history without requiring the user's global Git identity.
        run("git", "add", ".", cwd=site)
        run("git", "-c", "user.name=kbdiag docs initializer", "-c",
            "user.email=kbdiag-docs@users.noreply.github.com", "commit", "--quiet",
            "-m", "docs: initialize from OINK Starter", cwd=site)
        if args.check:
            check(site)
        if root.exists() or root.is_symlink():
            raise RuntimeError("目标在初始化期间被创建，已停止以保护现有文件。")
        site.rename(root)
    instructions(root, args.owner, args.repo)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, subprocess.CalledProcessError) as error:
        print(f"错误：{error}", file=sys.stderr)
        sys.exit(1)
