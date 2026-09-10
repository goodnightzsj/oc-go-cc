# 既有部署流程核对

仅提取本项目 Claude session 中的部署命令，不读取凭证或其它项目 session。来源：`~/.claude/projects/-Users-zsj-code-program-oc-go-cc/8ebffb0f-fb58-4290-b33e-b8345253d709.jsonl`（最后修改 2026-09-04）；例如 3039 行有 `git push origin main`，3041 行有 `bash scripts/prod-deploy.sh`，640/645 行同时记载远端目录与健康检查。与 `llmdoc/startup.md:21`、`scripts/prod-deploy.sh` 一致。

1. 本地验证、审查并提交本任务改动，`git push origin main`。
2. `ssh root@23.80.89.173`，在 `/root/oc-go-cc` 检查分支/工作区后 `git pull --ff-only origin main`。
3. `bash scripts/prod-deploy.sh` 构建带提交元数据的 release，同步模型 catalog，切换 `.tmp/prod/current` 并重启 `oc-go-cc.service`。
4. 检查 `systemctl is-active oc-go-cc.service`、`http://127.0.0.1:3456/health`、运行二进制及实际 GUI/API。

脚本在健康检查失败时将 current 链接切回之前的 release 并重启；成功后保留最近 5 个 release。配置和 SQLite 不是 release 内容，必须独立保留。SSH 使用既有 OpenSSH 认证，不读取私钥、不关闭主机密钥校验；发现不可信指纹或远端冲突则停止。

此记录仅说明已核实流程，不代表部署已执行。
