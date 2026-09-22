# WorkBuddy 任务提示词：codearts2api Docker 升级到 v0.1.6

> 用途：把本文件内容整段粘贴给 WorkBuddy 执行。任务范围**仅限 Docker 部署形态**。
> 供 WorkBuddy 阅读的项目事实见下文「二、环境事实」，均已由外部核实，无需重新摸索。

---

## 一、任务目标

把腾讯云轻量实例上现有的 **Docker 部署** codearts2api 原地升级到 `v0.1.6`：

1. 保留全部账号凭证与配置（`auths/`、`data/`、`config.json` 均不得丢失或改写内容）
2. （可选）确认 `update_repo` —— 自 `v0.1.5` 起已有内置默认值，无需再手工补
3. 逐项验收并原样回贴输出

**本任务不需要引入 systemd，不需要改动部署形态。** 该实例当前就是 Docker，升级方式就是拉取新代码后重建镜像。

---

## 二、环境事实（已核实）

| 项 | 值 |
| --- | --- |
| 实例 | 腾讯云轻量应用服务器 `lhins-ceumt1mw`，北京三区（ap-beijing-3） |
| 公网 / 内网 | `62.234.222.19` / `10.2.0.16` |
| 主机名 / 系统 | `VM-0-16-opencloudos`，OpenCloudOS 9.6，x86_64，当前以 root 操作 |
| 部署方式 | **Docker**（7866 端口由 `docker-proxy` 暴露） |
| 项目目录 | `/opt/codearts2api` |
| 运行进程 | `codearts2api -config config.json`（容器内） |
| 上游仓库 | https://github.com/hj01857655/codearts2api |
| 可用 Release | `v0.1.0`（旧）、`v0.1.1`、`v0.1.2`、`v0.1.3`、`v0.1.4`、`v0.1.5`、**`v0.1.6`（本次目标）**；`v0.1.2` 起带 Docker 版本注入，`v0.1.3` 起带新控制台面板，`v0.1.5` 起修掉一批模型发现与面板读数缺陷 |

现状问题（本次要修的）：

- 现有代码是旧版：升级前那份 `Dockerfile` 构建时**未注入版本号**，所以容器里 `codearts2api -version` 永远报 `dev`、`/status` 的 `version` 字段为空
  （该缺陷已在 `v0.1.2` 修复：`Dockerfile` 新增 `VERSION`/`COMMIT`/`BUILD_DATE` 构建参数，`docker-compose.yml` 透传。**`v0.1.1` 及更早不含此修复**；本次升到 `v0.1.6` 同样包含它）
- `v0.1.4` 及更早的版本里 `update_repo` **没有内置默认值**（只写在示例文件里），所以 `config.json` 缺这一项时控制台「检测更新」只会显示「未配置在线更新」。**自 `v0.1.5` 起默认已指向本项目 Release**，不配置也能用；只有显式写成 `""` 才关闭
- `/opt/codearts2api/config.json` 里的注释写着 `max_concurrent`「默认 5」，属旧文档错误（代码内置默认是 1），升级后会随新模板纠正

---

## 三、红线约束

1. **不得执行 `docker compose down -v`**。`auths/`、`data/` 是宿主机绑定卷，`-v` 会删除数据卷。
2. **不得改写 `auths/` 与 `data/` 下的任何文件**，也不得删除 `/opt/codearts2api/auths`。
3. `config.json` 是只读挂载进容器的，只能在**宿主机** `/opt/codearts2api/config.json` 上修改。
4. 该文件内含 `//` 注释，这是程序支持的格式（加载时会剥离），**修改时保留原有注释，不要为了「合法 JSON」把注释删掉**。
5. **不要把 `api_key` 的值打印到日志或回复里**，需要在命令里用变量引用。
6. 切分支前若 `git status` 有本地改动，**先说明有哪些改动再决定**，不要盲目 `git checkout -f` 覆盖。

---

## 四、执行步骤

### 步骤 1：只读核对现状

```bash
cd /opt/codearts2api
git remote -v
git log --oneline -1
git status --short
git tag | tail -5
docker compose ps
```

判断要点：

- 如果 `origin` 指向的是上游 `HITZY2002/codearts2api` 而不是 `hj01857655/codearts2api`，则需要增加远端后再拉取——`v0.1.6` 只存在于 `hj01857655` 这个 fork 上：
  ```bash
  git remote add fork https://github.com/hj01857655/codearts2api.git
  git fetch --tags fork
  ```
- **机器拉不到 github.com 时的备选方案**：本机实测过 `git fetch` 报 `Failed to connect to github.com port 443`（大陆机房常见）。若遇到同样情况，不要反复重试，改用镜像加速或让操作者从外部把仓库传到机器上（如 `git bundle` / `rsync`）。**不要**改为去 `git clone` 一个来源不明的镜像仓库。
- 如果 `git status --short` 列出 `config.json`，那是正常的（该文件被 `.gitignore` 忽略，属未跟踪）。其余改动请先报告。

### 步骤 2：拉取 v0.1.6 并重建镜像

```bash
cd /opt/codearts2api
git fetch --tags origin        # 或 fork，见步骤 1
git checkout v0.1.6

# 传入版本号后再构建：Dockerfile 已支持 VERSION/COMMIT/BUILD_DATE 构建参数，
# 不传则容器内二进制报 dev（与 Releases 的版本号对不上）。
export CA2A_BUILD_VERSION=$(git describe --tags --always)
export CA2A_BUILD_COMMIT=$(git rev-parse --short HEAD)
export CA2A_BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "将注入：$CA2A_BUILD_VERSION / $CA2A_BUILD_COMMIT / $CA2A_BUILD_DATE"

docker compose up -d --build
docker compose ps
docker compose logs --tail=30
```

说明：

- 镜像是多阶段构建（`golang:1.24-alpine` 构建 → `alpine:3.20` 运行），首次构建需拉取基础镜像，北京机房可能较慢，请等待完成后再判断。
- 当前使用旧版 `docker-compose.yml` 时也请拉取新代码后再构建（步骤 2 的 `git checkout` 已完成）；版本注入所需的 `build.args` 只在 `v0.1.2` 起才有。

### 步骤 3：（可选）确认 update_repo

`v0.1.5` 起 `update_repo` 已有内置默认值（`hj01857655/codearts2api`），**不配置也能检测更新**，本步骤可跳过。若要显式写进宿主机 `/opt/codearts2api/config.json`（注意逗号位置，保持文件内原有注释）：

```jsonc
"update_repo": "hj01857655/codearts2api",
```

改完后重启容器使配置生效（配置在启动时读取）：

```bash
docker compose restart
docker compose ps
```

### 步骤 4：验收

```bash
# 4.1 容器内二进制版本（期望输出 v0.1.6，含 commit 与构建时间）
docker compose exec codearts2api codearts2api -version

# 4.2 版本字段已注入（/status 需要鉴权；密钥从配置读取，不打印）
KEY=$(sed -nE 's/.*"api_key"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p' /opt/codearts2api/config.json | head -1)
curl -s -H "Authorization: Bearer $KEY" http://127.0.0.1:7866/status

# 4.3 检测更新端点
curl -s -H "Authorization: Bearer $KEY" http://127.0.0.1:7866/admin/api/update/check

# 4.4 控制台总览（accounts 数组长度 = 账号数，另有 stats.total 可直接看）
curl -s -H "Authorization: Bearer $KEY" http://127.0.0.1:7866/admin/api/overview

# 4.5 健康检查（免鉴权）
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7866/healthz
```

---

## 五、验收标准

| 检查项 | 期望结果 |
| --- | --- |
| `codearts2api -version` | `codearts2api v0.1.6 (commit ..., built ...)`（不再是 `dev`） |
| `docker compose ps` | 容器 `Up`，7866 映射正常 |
| `/status` 的 `version` | 不再是空值，为 `v0.1.6` |
| 控制台顶栏 | 齿轮右侧有用户菜单按钮（显示「已登录」/「未设置密钥」），点开含当前版本与退出登录 |
| `/status` 的账号数 | 与升级前一致（**不得减少**，账号数减少说明凭证卷出问题，立即停止并报告）；可用 `/admin/api/overview` 的 `stats.total` 快速核对 |
| 模型列表 | 面板「模型」页的「重新获取」点击后显示「已重新发现 N 个模型」（这是 `v0.1.5` 新接的真刷新端点，会绕过 1h 缓存直打上游）；列表数量**不应少于升级前**——`v0.1.5` 修掉了「主 agent 无模型就丢掉整路来源」的缺陷，通常会更全 |
| 福利额度页表头 | 为「今日额度 / 今日已用 / 今日剩余 / 剩余占比 / 本月已用」（`v0.1.5` 起区分日/月两个维度；旧版表头写着「总额度」，实际是当日值） |
| 容器日志 | 无 `load config` / `load auths` 之类启动失败 |
| `/healthz` | 有账号时 200；无账号时 503（属正常设计，不是故障） |
| `/admin/api/update/check` | 明确提示**容器内不支持在线更新，需改用镜像升级**（见下方说明） |

**关于「检测更新」的预期结果，务必注意**：本项目的在线更新功能设计上**只在非容器环境生效**，容器内会被明确拒绝，理由是容器里替换二进制会在下次重建时被镜像内容覆盖，属白做。因此这里返回「不支持 / 请改用镜像升级」类提示是**正确行为，不是故障，不要试图绕过或修改**。（`v0.1.5` 的提示词曾错误地写成 `docker compose pull && docker compose up -d`，但本项目没有镜像仓库发布链路，该命令会报找不到镜像；`v0.1.6` 起已修正为上一节的拉源码重建路径，`update/check` 会直接给出该命令。）

Docker 形态下的升级路径永远是重建镜像，本次任务执行的正是这条路径。

---

## 六、回滚预案

若升级后服务无法启动，用 Docker 原生方式退回上一版：

```bash
cd /opt/codearts2api
git checkout v0.1.1
unset CA2A_BUILD_VERSION CA2A_BUILD_COMMIT CA2A_BUILD_DATE   # v0.1.1 的 Dockerfile 不声明这些 ARG
docker compose up -d --build
docker compose logs --tail=30
```

注意：`v0.1.1` 的 `Dockerfile` 没有 `ARG`，所以回滚后 `-version` 会重新变成 `dev`，这是预期的。如果要回到升级前那个镜像，用 `docker images` 找到原镜像 ID，把 `docker-compose.yml` 的 `image:` 改成它（或直接 `docker run`）。

---

## 七、报告要求

每一步都贴**原始命令输出**，不要总结成「已成功」了事，也不要改写 JSON。至少包含：

1. 步骤 1 的 `git remote -v` / `git log --oneline -1` / `git status --short` 原始输出
2. 步骤 2 的 `docker compose up -d --build` 完整输出与 `docker compose ps`
3. 步骤 4 的四条验收命令原始输出（`update/check` 的 JSON 请原样粘贴）
4. 升级前后账号数对比

任一步失败，原样贴出报错，并附：

```bash
docker compose logs --tail=80
journalctl -n 50 --no-pager
```

**不要**在失败时自行改配置、改单元文件或重装环境；先报告，等指示。

---

## 八、另需确认（执行前必查）

**实例到期时间**：先前读到该实例到期日为 **2026-09-22，即本文件更新当日**。该日期来自此前一次人工核对，**未经计费接口复验**（本机访问不到腾讯云计费 API），所以执行前必须先确认：

- 在腾讯云控制台核对 `lhins-ceumt1mw` 的真实到期日与续费状态；
- 若已停机或即将停机，**先续费再执行本任务**——停机时 `docker compose up -d --build` 会失败，中途停机还可能留下半构建的镜像；
- 若已续费到新日期，把本节日期改成续费后的真实到期日再交接。
