# CodeArts2API

> 华为云 CodeArts Agent（盘古助手/码道）的 OpenAI 兼容代理。**无需运行 CodeArts Agent
> 客户端**，纯 Go 直连华为云 API，多账号轮转 + token 自动续期 + Web 管理控制台。

## 参考项目

本项目是 [Sliverkiss](https://github.com/Sliverkiss) 同系列开源项目的延伸实现，架构与运维形态参考了以下仓库：

- [workbuddy2api](https://github.com/Sliverkiss/workbuddy2api) — WorkBuddy CN OpenAI 兼容反代（账号池 / 轮转 / 签到架构）
- [traework2api](https://github.com/Sliverkiss/traework2api) — TRAE Work OpenAI 兼容反代（零依赖 Go 骨架）
- [qoderwork2api](https://github.com/Sliverkiss/qoderwork2api) — QoderWork CN OpenAI 兼容反代（OAuth 授权流程）

感谢原作者的开源与优秀设计。

本仓库是本项目在 GitHub 上的个人维护分支：[hj01857655/codearts2api](https://github.com/hj01857655/codearts2api)。

## 快速开始（Ubuntu / Linux）

```bash
make linux            # bin/ 下 4 个 Linux 静态二进制
make test
```

### 登录（华为云账号）

```bash
# 本机有浏览器
./login.sh

# 服务器（无浏览器）：打印链接，任意机器浏览器打开，ticket 轮询下发
./login.sh -print-only

# 凭证落盘 auths/codearts-{user_id}.json
```

### 启动

```bash
cp config.example.json config.json
export CA2A_API_KEY=你的随机密钥
./bin/codearts2api -config config.json
```

### 验证 + 管理控制台

```bash
curl http://127.0.0.1:7866/healthz
curl http://127.0.0.1:7866/v1/models -H "Authorization: Bearer $CA2A_API_KEY"
curl -X POST http://127.0.0.1:7866/v1/chat/completions \
  -H "Authorization: Bearer $CA2A_API_KEY" -H "Content-Type: application/json" \
  -d '{"model":"snap-chat","messages":[{"role":"user","content":"你好"}]}'
```

浏览器打开 **http://127.0.0.1:7866/** 即管理控制台（面板经 `//go:embed` 打进二进制，无需单独部署前端）。
多轮上下文按账号自动续接（chat_id 分组）；也可用请求头
`X-Codearts-Chat-Id: <chatId>` 或 body 里 `conversation_id` 显式指定会话。

### Web 控制台

左侧导航分五个视图，顶栏右侧是动作区（刷新、设置菜单）：

| 视图 | 内容 |
|------|------|
| 账号池 | 账号健康色条与状态（可用/冷却/禁用）、token 剩余、在途会话、故障计数与原因；逐账号刷新状态 / 保活 / 启用 / 禁用，以及全员刷新、全员保活、重载 auths。**授权登录**在表头右上角 |
| 福利额度 | 各账号限时福利签到时间、总额度 / 已用 / 剩余与占比；支持刷新额度、全部领取 |
| 模型 | 当前可用模型（已过滤探测判定不可用的条目），点击标签复制模型 ID；下方是接入地址 |
| 调度 | 后台巡检与保活参数回显（轮询间隔、提前刷新窗口、保活间隔与窗口、单账号最大在途） |
| 操作记录 | 本页发出的动作与结果流水，可筛操作 / 错误 |

- **登录**：默认直接尝试加载（经反向代理并注入 `api_key` 时无需手填）；直连且未带 key 时回退到密钥表单，密钥只存本机浏览器 `localStorage`，请求以 `Authorization: Bearer` 发送。设置菜单里的**退出登录**会清除本地密钥并回到表单。
- **主题**：明暗共四套色板（石墨、午夜、浅色、暖沙），在设置菜单里切换，选择存在浏览器本地；所有文字与状态色都按 WCAG AA（正文 4.5:1）取值。
- **授权登录**：点「＋ 授权登录」获取链接，浏览器完成华为云登录后账号自动落盘并载入账号池，无需重启；部分浏览器会停在 `127.0.0.1` 回调，此时把地址栏完整地址粘贴进弹窗的「远程浏览器回调中转」即可。
- **冷却**：只按时间自动恢复，面板没有手动清除按钮；有账号冷却时面板会定期补一次读数，到期自动回到可用态。
- **在线更新**：设置菜单里有「版本」分组（当前版本 + 检测更新 / 立即更新 / 回滚 / 重启）。详见 [在线更新](#在线更新) 与 [docs/online-update.md](docs/online-update.md)

### 在线更新

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

`codearts2api -version` 可随时确认当前二进制版本；`GET /status` 也带 `version` 字段。

详细设计与取舍见 [docs/online-update.md](docs/online-update.md)。

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

### 模型列表与限时福利

`/v1/models` 返回上游**精确模型 ID**（区分大小写，如 `GLM-5.2`、`Qwen3-VL-235B`），
同时为含大写的 ID 补一条小写别名（`glm-5.2`），两者都能用于聊天。限时福利
（免费套餐）模型额外带 `benefit: true` 标记，聊天时服务端会自动追加上游要求的
`maas_type: benefit` 请求头（按发起请求的账号判定，多账号套餐不同也不会串——
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

### API Key 必填

服务没有 API Key 会**拒绝启动**（旧版本会回退到公开的 `dummy-key-for-codearts`，
等于把接口暴露给任何人）。三选一：

```bash
# 1) env（推荐，配合 .env / systemd EnvironmentFile）
export CA2A_API_KEY=$(openssl rand -hex 24)
# 2) config.json 的 "api_key" 字段
# 3) docker compose 的 .env
```

### 账号续期

`refresh_token` 与登录时的 `client_id`、DPoP 私钥绑定，因此凭证文件会保存
`client_id` 与 `dpop_private_key`；刷新时原样复用并写回轮转后的新 refresh_token。
若手工换过 `login_client_id`，老账号仍按自己记录的 client_id 刷新。

## 部署（systemd / Docker）

```bash
sudo mkdir -p /opt/codearts2api && sudo cp -r bin config.example.json auths /opt/codearts2api/
sudo cp deploy/codearts2api.service /etc/systemd/system/
# 编辑 /opt/codearts2api/.env 写 CA2A_API_KEY，改好 config.json
sudo systemctl daemon-reload && sudo systemctl enable --now codearts2api

# 或 Docker
export CA2A_API_KEY=你的随机密钥
mkdir -p auths data
docker compose up -d --build
```

## 环境变量配置

除了 `config.json`，还支持以下环境变量覆盖：

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `CA2A_API_KEY` | API 访问密钥 | - |
| `CA2A_LISTEN` | 监听地址 | `:7866` |
| `CA2A_AUTH_DIR` | 凭证目录 | `./auths` |
| `CA2A_STATE_FILE` | 状态文件 | `./data/state.json` |
| `CA2A_DEFAULT_MODEL` | 默认模型 | `glm-5.2` |
| `CA2A_OAUTH_CALLBACK_HOST` | OAuth 回调主机 | - |
| `CA2A_WATCH_ENABLED` | 调度器开关 | `true` |
| `CA2A_WATCH_POLL_MINUTES` | 轮询间隔（分钟） | `30` |
| `CA2A_WATCH_REFRESH_SKEW` | 提前刷新时间（分钟） | `30` |
| `CA2A_WATCH_KEEPALIVE_INTERVAL` | 保活间隔（分钟） | `15` |
| `CA2A_MAX_CONCURRENT` | 单账号最大并发 | `5` |
| `CA2A_KEEPALIVE_WINDOW` | 保活窗口 | `10m` |
| `CA2A_QUEUE_RETRY_SECONDS` | 上游并发/TPM 排队时的重试间隔（秒） | `10` |
| `CA2A_QUEUE_MAX_ATTEMPTS` | 排队重试次数上限（约 5 分钟） | `30` |
| `CA2A_BENEFIT_AUTO_CLAIM` | 发现模型时自动领取限时福利（幂等；关闭则福利模型不可用） | `true` |
| `CA2A_LOGIN_CLIENT_ID` | 控制台授权登录使用的 OAuth client_id | 已有账号的取值，否则 `codearts-agent` |
| `CA2A_UPDATE_REPO` | 在线更新使用的 GitHub 仓库（owner/name） | 见 `config.json` 的 `update_repo` |

## 目录结构

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

## 免责声明

仅供学习和研究使用。使用者需遵守华为云服务条款，自行承担使用风险。

## License

MIT

