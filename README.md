<div align="center">

# CodeArts2API

**把华为云 CodeArts Agent 账号变成 OpenAI 兼容 API 的多账号网关**

无需运行 CodeArts Agent 客户端 · 纯 Go 直连华为云 API · 多账号轮转 · token 自动续期 · Web 管理控制台

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white&style=flat-square)](go.mod)
[![API](https://img.shields.io/badge/API-OpenAI_Compatible-412991?style=flat-square)](#api)
[![Platform](https://img.shields.io/badge/Platform-linux%20amd64%20%2F%20arm64-333333?style=flat-square)](#部署systemd--docker)
[![Release](https://img.shields.io/github/v/release/hj01857655/codearts2api?style=flat-square&color=2496ED)](https://github.com/hj01857655/codearts2api/releases)
[![License](https://img.shields.io/github/license/hj01857655/codearts2api?style=flat-square&color=green)](LICENSE)

</div>

---

CodeArts2API 是一个自托管的 **OpenAI 兼容上游网关**：把 CodeArts Agent（盘古助手 / 码道）
账号包装成标准的 `/v1/chat/completions` 与 `/v1/models`，现有 OpenAI SDK、前端与工具
**零改造接入**。网关侧负责账号池调度、token 自动续期、冷却熔断与管理控制台。

## 目录

- [特性](#特性)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [API](#api)
- [Web 控制台](#web-控制台)
- [在线更新](#在线更新)
- [使用说明](#使用说明)
- [部署（systemd / Docker）](#部署systemd--docker)
- [项目结构](#项目结构)
- [相关文档](#相关文档)
- [致谢](#致谢)
- [免责声明](#免责声明)
- [License](#license)

## 特性

- **OpenAI 兼容** — `POST /v1/chat/completions`（流式 / 非流式）、`GET /v1/models`、
  `GET /v1/models/{id}`；流式按 OpenAI SSE 协议输出，非流式带 `usage`。
- **多账号池** — 账号健康状态（可用 / 冷却 / 禁用）、单账号并发限制、故障自动换号与冷却，
  避免单号过载与雪崩。
- **token 自动续期** — 后台看门狗按 `refresh_skew_minutes` 提前刷新，另按保活窗口发送心跳；
  续期写回轮转后的 `refresh_token`。
- **会话续接** — 多轮上下文按账号自动续接（chat_id 分组），也可用请求头
  `X-Codearts-Chat-Id` 或 body 的 `conversation_id` 显式指定。
- **授权登录免重启** — 控制台内点「授权登录」完成华为云 OAuth，账号落盘后自动载入池。
- **限时福利自动领取** — 福利（免费套餐）模型在模型发现时幂等领取，聊天时自动附加上游要求的
  `maas_type: benefit` + `model-id` + `model-name` 三个头（按发起请求的账号判定）。
- **模型可用性探测** — 上游模型目录会列出账号实际未注册的模型，服务周期性探测并过滤，
  只暴露真实可用的条目。
- **Web 管理控制台** — 面板经 `//go:embed` 打进二进制，账号池 / 福利额度 / 模型 / 调度 /
  操作记录五个视图，明暗四套色板。
- **在线更新** — 控制台一键检测 / 升级 / 回滚，只发一个二进制（见 [在线更新](#在线更新)）。
- **零第三方依赖** — 只用 Go 标准库（无 `go.sum`），单文件部署。

## 快速开始

### 环境要求

- 一个或多个华为云账号，用于 OAuth 登录
- Go ≥ 1.22（仅源码构建需要）；运行时不依赖任何第三方库
- Linux（amd64 / arm64）可直装自更新；Docker 部署见[部署](#部署systemd--docker)

### 1. 获取二进制

```bash
# 从 Release 下载（linux/amd64；arm64 换 codearts2api_linux_arm64.tar.gz）
mkdir -p bin
curl -fsSL https://github.com/hj01857655/codearts2api/releases/latest/download/codearts2api_linux_amd64.tar.gz \
  | tar -xz -C bin/ codearts2api
```

或从源码构建：

```bash
make linux            # bin/ 下 4 个 Linux 静态二进制
make windows          # bin/ 下 4 个 Windows 二进制
make test             # 跑全部测试
```

### 2. 准备配置

```bash
cp config.example.jsonc config.json
```

密钥三选一（详见[配置说明](#配置说明)）；没有密钥服务会**拒绝启动**：

```bash
export CA2A_API_KEY=$(openssl rand -hex 24)
```

### 3. 登录账号

```bash
# 本机有浏览器
./login.sh

# 服务器（无浏览器）：打印链接，任意机器浏览器打开，ticket 轮询下发
./login.sh -print-only

# 凭证落盘 auths/codearts-{user_id}.json（0600），可重复执行添加多账号
```

### 4. 启动并验证

```bash
./bin/codearts2api -config config.json
```

另一个终端验证：
curl http://127.0.0.1:7866/healthz
curl http://127.0.0.1:7866/v1/models -H "Authorization: Bearer $CA2A_API_KEY"
curl -X POST http://127.0.0.1:7866/v1/chat/completions \
  -H "Authorization: Bearer $CA2A_API_KEY" -H "Content-Type: application/json" \
  -d '{"model":"snap-chat","messages":[{"role":"user","content":"你好"}]}'
```

浏览器打开 **http://127.0.0.1:7866/** 即管理控制台（面板经 `//go:embed` 打进二进制，无需单独部署前端）。
多轮上下文按账号自动续接（chat_id 分组）；也可用请求头
`X-Codearts-Chat-Id: <chatId>` 或 body 里 `conversation_id` 显式指定会话。

`codearts2api -version` 可随时确认当前二进制版本；`GET /status` 也带 `version` 字段。

命令行的其他子命令（`login` / `credit` / `apply` / `models` / `benefit` / `probe` /
`chatdebug`）见[项目结构](#项目结构)与各自的 `go run ./cmd/<name> -h`。

## 配置说明

### 环境变量

除 `config.json` 外，以下环境变量可覆盖同名配置：

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `CA2A_API_KEY` | API 访问密钥（**必填**，缺失则拒绝启动） | - |
| `CA2A_LISTEN` | 监听地址 | `:7866` |
| `CA2A_AUTH_DIR` | 凭证目录 | `./auths` |
| `CA2A_STATE_FILE` | 状态文件 | `./data/state.json` |
| `CA2A_DEFAULT_MODEL` | 默认模型 | `glm-5.2`（`config.example.jsonc` 里给的是 `snap-chat`） |
| `CA2A_OAUTH_CALLBACK_HOST` | OAuth 回调主机 | - |
| `CA2A_WATCH_ENABLED` | 调度器开关 | `true` |
| `CA2A_WATCH_POLL_MINUTES` | 轮询间隔（分钟） | `30` |
| `CA2A_WATCH_REFRESH_SKEW` | 提前刷新时间（分钟） | `30` |
| `CA2A_WATCH_KEEPALIVE_INTERVAL` | 保活间隔（分钟） | `15` |
| `CA2A_MAX_CONCURRENT` | 单账号最大并发 | `1`（串行最稳；示例配置里为 `5`） |
| `CA2A_KEEPALIVE_WINDOW` | 保活窗口 | `10m` |
| `CA2A_QUEUE_RETRY_SECONDS` | 上游并发/TPM 排队时的重试间隔（秒） | `10` |
| `CA2A_QUEUE_MAX_ATTEMPTS` | 排队重试次数上限（约 5 分钟） | `30` |
| `CA2A_BENEFIT_AUTO_CLAIM` | 发现模型时自动领取限时福利（幂等；关闭则福利模型不可用） | `true` |
| `CA2A_LOGIN_CLIENT_ID` | 控制台授权登录使用的 OAuth client_id | 已有账号的取值，否则 `codearts-agent` |
| `CA2A_UPDATE_REPO` | 在线更新使用的 GitHub 仓库（owner/name；空字符串关闭） | `hj01857655/codearts2api` |

### config.json

`config.example.jsonc` 可直接复制使用（`//` 与 `/* */` 注释会被忽略）：

```jsonc
{
  "api_key": "",                  // 必填，或用 CA2A_API_KEY
  "listen": ":7866",
  "auth_dir": "./auths",
  "state_file": "./data/state.json",
  "default_model": "snap-chat",
  "cooldown": { "soft_rate": "60s", "err_threshold": 3, "err_cooldown": "10m" },
  "benefit_auto_claim": true,
  "update_repo": "hj01857655/codearts2api",   // 默认已指向本项目 Release；显式写成 "" 才关闭在线更新
  "max_concurrent": 5,
  "keepalive_window": "10m",
  "watch": { "enabled": true, "poll_minutes": 30, "refresh_skew_minutes": 30,
             "keepalive_interval_minutes": 15 },
  "upstream": { "timeout_seconds": 120 }
}
```

## API

除免鉴权端点（`GET /healthz`、面板页面 `/` `/admin` `/panel` `/panel/`、
`GET /oauth/callback`）外，其余端点都要求 `Authorization: Bearer <api_key>`。

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/chat/completions` | OpenAI 兼容对话（`stream` 可选） |
| `GET` | `/v1/models` | 可用模型列表（含限时福利标记） |
| `GET` | `/v1/models/{id}` | 单个模型详情 |
| `GET` | `/healthz` | 健康检查（无可用账号时 503，免鉴权） |
| `GET` | `/status` | 账号池与版本状态 |
| `GET` | `/` `/admin` `/panel` `/panel/` | 管理控制台页面（免鉴权） |
| `GET` | `/admin/api/overview` | 控制台总览 |
| `POST` | `/admin/api/credits` | 刷新额度读数 |
| `POST` | `/admin/api/checkin` | 限时福利签到 |
| `GET` | `/admin/api/benefit/status` | 福利额度状态 |
| `POST` | `/admin/api/keepalive` | 手动保活 |
| `POST` | `/admin/api/reload` | 重载 `auths/` 目录 |
| `POST` | `/admin/api/accounts/enable` `/disable` `/clear-cooldown` | 账号启用 / 禁用 / 清冷却 |
| `POST` | `/admin/api/oauth/start` `/poll` `/import-callback` | 控制台授权登录 |
| `GET` | `/oauth/callback` | OAuth 回调（免鉴权） |
| `GET` | `/admin/api/update/check` | 检测新版本 |
| `POST` | `/admin/api/update/apply` `/rollback` `/restart` | 升级 / 回滚 / 重启 |

## Web 控制台

左侧导航分五个视图，顶栏右侧是动作区（刷新、设置菜单、用户菜单）：

| 视图 | 内容 |
|------|------|
| 账号池 | 账号健康色条与状态（可用/冷却/禁用）、token 剩余、在途会话、故障计数与原因；逐账号刷新状态 / 保活 / 启用 / 禁用，以及全员刷新、全员保活、重载 auths。**授权登录**在表头右上角 |
| 福利额度 | 各账号限时福利签到时间、总额度 / 已用 / 剩余与占比；支持刷新额度、全部领取 |
| 模型 | 当前可用模型（已过滤探测判定不可用的条目），点击标签复制模型 ID；下方是接入地址 |
| 调度 | 后台巡检与保活参数回显（轮询间隔、提前刷新窗口、保活间隔与窗口、单账号最大在途） |
| 操作记录 | 本页发出的动作与结果流水，可筛操作 / 错误 |

- **登录**：默认直接尝试加载（经反向代理并注入 `api_key` 时无需手填）；直连且未带 key 时回退到密钥表单，密钥只存本机浏览器 `localStorage`，请求以 `Authorization: Bearer` 发送。顶栏最右的**用户菜单**显示密钥状态（已登录 / 未设置密钥），点开可看当前版本并**退出登录**（清除本地密钥并回到表单）。
- **用户菜单**：位于设置菜单右侧，收起态显示登录状态，展开后是密钥是否已保存 + 当前版本 + 退出登录；带 `aria-haspopup` / `aria-expanded`，与设置菜单互斥，`Esc` 或点击外部关闭。
- **主题**：明暗共四套色板（石墨、午夜、浅色、暖沙），在设置菜单里切换，选择存在浏览器本地；所有文字与状态色都按 WCAG AA（正文 4.5:1）取值。
- **授权登录**：点「＋ 授权登录」获取链接，浏览器完成华为云登录后账号自动落盘并载入账号池，无需重启；部分浏览器会停在 `127.0.0.1` 回调，此时把地址栏完整地址粘贴进弹窗的「远程浏览器回调中转」即可。
- **冷却**：只按时间自动恢复，面板没有手动清除按钮；有账号冷却时面板会定期补一次读数，到期自动回到可用态。
- **在线更新**：设置菜单里有「版本」分组（当前版本 + 检测更新 / 立即更新 / 回滚 / 重启）。详见 [在线更新](#在线更新) 与 [docs/online-update.md](docs/online-update.md)

## 在线更新

部署后的实例可在控制台设置菜单（顶栏齿轮）里点「检测更新」，从 GitHub Releases 拉新版：
下载当前平台归档 → 校验 SHA256 → 原子替换二进制 → 你确认后重启生效。

```jsonc
"update_repo": "hj01857655/codearts2api"   // 或 CA2A_UPDATE_REPO；留空则关闭在线更新
```

- 重启不调 `systemctl`（那需要 sudo），而是进程自行退出，交给 unit 的 `Restart=always`
  拉起（`deploy/codearts2api.service` 已配置）。
- 替换前旧版会留在 `codearts2api.backup`，可随时「回滚上一版」。
- **只支持 Linux + systemd 直装**。容器内会被下次 `docker compose up -d` 覆盖，
  因此端点会直接拒绝并提示改用镜像；Windows 无法自替换运行中的文件。
- 只发 `cmd/server` 一个二进制（面板经 `//go:embed` 已打进它），`login`/`credit`/`apply`
  不参与在线更新。
- 发版：打 `v*` tag 触发 `.github/workflows/release.yml`（goreleaser），产出
  `codearts2api_<os>_<arch>.tar.gz` 与 `checksums.txt`。

详细设计与取舍见 [docs/online-update.md](docs/online-update.md)。

## 使用说明

### 模型列表与限时福利

`/v1/models` 返回上游**精确模型 ID**（区分大小写，如 `GLM-5.2`、`Qwen3-VL-235B`），
同时为含大写的 ID 补一条小写别名（`glm-5.2`），两者都能用于聊天。限时福利
（免费套餐）模型额外带 `benefit: true` 标记，聊天时服务端会自动追加上游要求的
`maas_type: benefit` + `model-id` + `model-name` 请求头（按发起请求的账号判定，多账号套餐不同也不会串——
列表是各账号可用模型的并集，实际路由会优先挑目录里真有这个模型的账号）。

福利模型列表随免费套餐轮换，用下面这条命令核对当前账号实际可用的模型：

```bash
go run ./cmd/models                 # 账号可用模型（内置 + 福利）
go run ./cmd/models -json           # 机器可读
go run ./cmd/models -claim          # 先领取限时福利再查询（幂等，属写操作）
```

限时福利模型**必须先领取才能调用**：未领取时上游一律返回
`InferHub.4004.200 benefit not found`（实测）。领取是幂等操作，官方客户端打开模型
菜单时也会调用，因此服务默认会领取（`benefit_auto_claim: true`）。不想让服务写账号
可显式关闭：

```jsonc
"benefit_auto_claim": false   // 或 CA2A_BENEFIT_AUTO_CLAIM=0
```

关闭后福利模型仍会出现在 `/v1/models`，但调用必然失败。

### 模型可用性探测

`/v1/models` 只列**真实可用**的模型：上游的模型发现（agent-center / builtin / 福利
网关）会列出当前账号根本没注册的模型（实测 `GLM-5.2-ArkTS-SPARK`、`OpenPangu-2.0-Pro`
等返回 `InferHub.002002009.404`）。服务会周期性用一条最小请求探测并缓存结论，
真实请求失败也会被学习（`not registered` / `benefit not found` 记为账号能力问题，
不会给健康账号记错误冷却）。探测结果缓存 30 分钟。

### 跨域与单模型查询

所有响应都带 `Access-Control-Allow-Origin: *`，`OPTIONS` 预检直接返回 `204`，
浏览器端前端可直连（密钥经 `Authorization: Bearer` 传入）。除 `/v1/models` 列表外，
还提供 `GET /v1/models/{id}` 返回单个模型详情——部分客户端会逐个查模型。

### 磁盘缓存

模型目录与多轮会话映射会在成功获取后落盘（`state_file + ".models.json"` /
`state_file + ".chats.json"`，即 `data/state.json.models.json` 与 `data/state.json.chats.json`），
启动时优先加载，避免重启后首请求重新发现模型、丢失会话续接。

### token 用量（usage）

非流式响应始终带 `usage`：**优先透传上游原生值**（含 `completion_tokens_details`
等扩展字段原样保留），上游未提供时才按文本长度估算——估算值不是精确 token 计数。

流式默认不返回 usage。需要用量时显式开启：

```jsonc
"stream_options": { "include_usage": true }
```

开启后在 `[DONE]` 前会多一个 `choices: []`、`usage` 带值的终帧（OpenAI 同约定），
其余增量 chunk 的 `usage` 均为 `null`；上游整段没给 usage 时该终帧用估算值兜底。
`stream_options` 未传、传 `null`、或 `include_usage: false` 均不返回 usage。
上游报错时只发 `event: error`，不发 usage。

### 账号续期

`refresh_token` 与登录时的 `client_id`、DPoP 私钥绑定，因此凭证文件会保存
`client_id` 与 `dpop_private_key`；刷新时原样复用并写回轮转后的新 refresh_token。
若手工换过 `login_client_id`，老账号仍按自己记录的 client_id 刷新。

## 部署（systemd / Docker）

二进制按平台从 Release 下载（或本地 `make linux` 产出 `bin/`）：

```bash
mkdir -p bin
curl -fsSL https://github.com/hj01857655/codearts2api/releases/latest/download/codearts2api_linux_amd64.tar.gz \
  | tar -xz -C bin/ codearts2api
```

### systemd

```bash
# 1. 目录、二进制与配置。auths/ 与 data/ 不在仓库里（.gitignore），必须自建：
sudo mkdir -p /opt/codearts2api/{bin,auths,data}
sudo cp bin/codearts2api /opt/codearts2api/bin/
sudo cp config.example.jsonc /opt/codearts2api/config.json
sudo cp deploy/codearts2api.service /etc/systemd/system/

# 2. 配置：写 api_key（或 .env 里的 CA2A_API_KEY），想让控制台能在线更新就填 update_repo
sudo sh -c 'umask 077; printf "CA2A_API_KEY=%s\n" "$(openssl rand -hex 24)" > /opt/codearts2api/.env'
sudo editor /opt/codearts2api/config.json   # api_key / update_repo

# 3. 授权与权限：User=ubuntu 需能写 auths/、data/ 与 bin/（自更新要替换二进制）
sudo chown -R ubuntu:ubuntu /opt/codearts2api
sudo systemd-analyze verify /etc/systemd/system/codearts2api.service   # 可选：先查 unit
sudo systemctl daemon-reload && sudo systemctl enable --now codearts2api
```

两个容易挡在首次启动前的细节：`auths/` 与 `data/` 必须先存在（unit 的
`ReadWritePaths` 指向不存在的路径会让服务以 `226/NAMESPACE` 失败），`bin/` 必须
属于运行用户（否则在线更新最后一步的 rename 会因权限失败）。

非 Ubuntu 的机器记得把 unit 里的 `User=ubuntu` 改成实际用户。

### Docker

```bash
export CA2A_API_KEY=你的随机密钥
mkdir -p auths data
cp config.example.jsonc config.json
docker compose up -d --build
```

容器内不支持在线更新（会被下次 `up --build` 覆盖），升级改用拉源码后重建：

```bash
cd /opt/codearts2api
git fetch --tags && git checkout <新版本 tag>
docker compose up -d --build
```

## 项目结构

```
cmd/server/        HTTP 服务（config + main）
cmd/login/         华为云 OAuth2 PKCE 登录
cmd/credit/        账号登录态日报（含并发信息）
cmd/apply/         批量 token 续期（使用 pool 包）
cmd/models/        查看账号可用模型（内置 + 限时福利，可选 -claim 领取）
cmd/benefit/       限时福利签到与余额查询
cmd/probe/         模型可用性探测
cmd/chatdebug/     上游对话调试
internal/auth/     auth 文件读写
internal/upstream/ 云端客户端（登录/聊天/SSE/模型发现）+ 逆向常量
internal/pool/     账号池（token 校验/自动刷新/冷却/并发控制）
internal/scheduler/ token 续期看门狗（含保活机制）
internal/update/    在线更新（检测/校验/原子替换/回滚，仅标准库）
internal/server/   OpenAI 兼容路由 + 管理控制台
                   panel.html / panel.go（内嵌面板）、admin.go（面板 API）、
                   admin_update.go（在线更新端点）、oauth.go（授权登录）
deploy/            systemd unit 样例
docs/              逆向记录与接口清单；在线更新设计说明（online-update.md）
```

## 相关文档

- [docs/online-update.md](docs/online-update.md) — 在线更新的设计、约束与实施记录
- [docs/reverse-engineering.md](docs/reverse-engineering.md) — CodeArts Agent 逆向记录与接口清单

## 致谢

本项目是 [HITZY2002/codearts2api](https://github.com/HITZY2002/codearts2api) 的个人维护分支
（[hj01857655/codearts2api](https://github.com/hj01857655/codearts2api)）。架构与运维形态参考了
[Sliverkiss](https://github.com/Sliverkiss) 的同系列开源项目：

- [workbuddy2api](https://github.com/Sliverkiss/workbuddy2api) — WorkBuddy CN OpenAI 兼容反代（账号池 / 轮转 / 签到架构）
- [traework2api](https://github.com/Sliverkiss/traework2api) — TRAE Work OpenAI 兼容反代（零依赖 Go 骨架）
- [qoderwork2api](https://github.com/Sliverkiss/qoderwork2api) — QoderWork CN OpenAI 兼容反代（OAuth 授权流程）

感谢原作者的开源与优秀设计。

## 免责声明

本项目（包括代码、脚本、文档与配置示例）**仅供个人学习与研究使用**，为非官方网关，
与华为云无任何隶属关系。

- 仅限使用**本人持有且已获授权的账号**，仅限本机 / 私有环境测试；不得共享、转售或违规分发。
- 使用本项目涉及目标平台服务条款与账号风险，账号封禁、条款违约或使用结果由使用者自行承担。
- `auths/` 内为明文凭证，请妥善保管；对外暴露端口前务必设置 `api_key` 并置于可信网络。
- 本项目按「现状」提供，不附带任何明示或默示的担保；使用所产生的风险与后果由使用者承担。
- 文中引用的第三方产品、服务与商标，其权利归各自权利人所有。

## License

[MIT](LICENSE) © 2026 HITZY2002

