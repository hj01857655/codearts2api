# 在线更新设计（Web 控制台「检测更新」）

> 状态：**已实现**。实现位置：`internal/update/`（更新服务）、`internal/server/admin_update.go`
> （四个端点）、`cmd/server/main.go`（版本注入）、`.goreleaser.yaml` 与
> `.github/workflows/release.yml`（发版）。本文描述设计与约束，代码与本文如有出入以代码为准。

## 1. 目标

在已部署实例的 Web 控制台点一个按钮，走完这条链路：

1. 比对 GitHub Releases，判断是否有新版本
2. 下载当前平台产物，校验 SHA256
3. 原子替换正在运行的二进制
4. 优雅退出，由 systemd 拉起新版本
5. 出问题时回滚到上一版

硬约束：不引入第三方自更新库；不需要 sudo；不破坏现有 systemd 与 Docker 部署。

## 2. 现状核查（实施前）

下表为开工前的状态，保留作对照；「缺口」一列在本轮已全部补齐（见第 8 节）。

| 能力 | 现状 | 缺口 |
| --- | --- | --- |
| 服务自身版本号 | `cmd/server/main.go` 无版本变量；`Makefile` 各目标带 `-buildvcs=false` 且无 `-ldflags` | 二进制不知道自己是哪一版，无从比对 |
| Git tag / Release | 无 tag、无 Release、无 `.github/workflows` | 更新器无「新版本」可发现 |
| 发布产物 | 手工 `make linux`，仅 4 个本地二进制 | 无 checksums，无按平台命名的 asset |
| 更新服务 | 无 | 核心缺口 |
| 面板入口 | 无 | 缺口 |
| systemd 自动重启 | **已有**：`deploy/codearts2api.service` 的 `Restart=always` / `RestartSec=5` | 无需改动，是「退出即重启」的前提 |
| 二进制目录可写 | **可以**：`ProtectSystem=full` 只读化 `/usr`、`/boot`、`/etc`，`/opt` 仍可写 | 若日后收紧为 `strict`，须把 `/opt/codearts2api/bin` 加入 `ReadWritePaths`（现仅 `auths`、`data`） |
| Docker 部署 | `config.json` 只读挂载，`auths`/`data` 为 volume | 容器内自更新会被重建覆盖，见 §7 |

注：`internal/upstream/client.go` 的 `plugin-version: 5.2.0` 是**上游华为的插件版本**，
与服务自身版本无关，不可作为比较基准。

## 3. 参考实现：sub2api 的更新链路

关键结论：**它没有用任何第三方自更新 package**。`backend/internal/service/update_service.go`
（约 19 KB）的 import 除自身的 `internal/pkg/errors` 外全是标准库：

```
archive/tar  bufio  compress/gzip  context  crypto/sha256  encoding/hex
encoding/json  fmt  io  net/url  os  path/filepath  runtime  sort
strconv  strings  time
```

链路四步：

1. **发版**：goreleaser 产出 `{GOOS}_{GOARCH}` 命名的归档与一份 `checksums.txt`，
   并用 ldflags 注入 `-X main.Commit` / `-X main.Date` / `-X main.BuildType=release`。
2. **检测**：查 GitHub Releases API 的 `/releases/latest`，与当前版本比较。
3. **应用**：按 `runtime.GOOS/GOARCH` 选 asset → 下载 → 校验 SHA256 → 解压出二进制 →
   `chmod 0755` → 原子替换。
4. **重启**：接口返回 `need_restart: true`；重启不是调 systemctl（需要 sudo），
   而是 `os.Exit(0)` 交给 systemd 的 `Restart=always` 拉起，且先 `time.Sleep(500ms)`
   保证 HTTP 响应已发出。

原子替换的具体顺序（这是整个机制的关键，必须照抄）：

```go
// 1. 当前二进制改名为备份（同目录同文件系统，rename 是原子的）
os.Rename(exePath, exePath+".backup")
// 2. 新二进制就位；失败则把备份换回去
if err := os.Rename(newBinaryPath, exePath); err != nil {
    _ = os.Rename(exePath+".backup", exePath)
    return err
}
```

回滚就是把 `.backup` 换回来；换文件由服务自己做，重启仍由 systemd 负责。

它还做了两件安全上必须抄的事：

- `validateDownloadURL`：只允许 HTTPS 且域名属于 GitHub（防 SSRF）。
- 下载体积上限 `maxDownloadSize`，避免被超大响应打爆磁盘。

## 4. 本项目的设计

### 4.1 版本注入

新增 `cmd/server/main.go` 的包级变量，由构建时注入：

```go
var (
    version   = "dev"   // -X main.version=...
    commit    = "none"  // -X main.commit=...
    buildDate = "unknown"
)
```

加一个 `-version` 开关，打印 `version commit buildDate` 后退出；
`/status` 也应带上版本，便于远程确认当前跑的是哪一版。

### 4.2 发布产物

用 goreleaser，要点：

- `builds` 只编 `./cmd/server`（面板已 `//go:embed` 进去，单文件即可自更新）；
  其余 `login`/`credit`/`apply` 可另出 asset，但不参与自更新。
- `ldflags` 注入 4.1 的三个变量。
- `archives.name_template` 用 `{GOOS}_{GOARCH}`，更新器按此匹配。
- `checksum.name_template: checksums.txt`。

打 `v*` tag 触发 workflow 发 Release。

### 4.3 更新服务

新增 `internal/update/`（纯标准库），接口：

```go
type Service struct { /* httpc, repo, currentVersion */ }

func (s *Service) Check(ctx) (*Release, bool, error)   // 最新版 + 是否有更新
func (s *Service) Apply(ctx, rel *Release) error       // 下载→校验→原子替换
func (s *Service) Rollback() error                     // .backup 换回
func (s *Service) Restart()                            // 延迟 os.Exit(0)
```

`Apply` 的步骤（每一步失败都要在替换前中止）：

1. 按 `runtime.GOOS/GOARCH` 在 assets 里找匹配归档，找不到就报错返回。
2. 校验下载 URL 协议与域名，仅放行 HTTPS + 白名单域名。
3. 下载到**与可执行文件同目录**的临时文件（保证后续 rename 不跨文件系统）。
4. 拉 `checksums.txt`，比对 SHA256；不一致立即中止。
5. 解压取二进制，`chmod 0755`。
6. 按第 3 节的顺序做原子替换，并在替换前把旧版留成 `.backup`。

### 4.4 HTTP 端点

全部挂在既有的 `withAuth` 之下（与其他 `/admin/api/*` 一致）：

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/admin/api/update/check` | 查最新版，返回当前版本 / 最新版本 / 是否有更新 / 发布说明 |
| POST | `/admin/api/update/apply` | 下载并替换二进制，返回「需重启」 |
| POST | `/admin/api/update/rollback` | 回滚到 `.backup` |
| POST | `/admin/api/update/restart` | 延迟退出，交由 systemd 拉起 |

注意：`apply` 是长时间操作（下载可达数十 MB），需要一个脱离请求上下文的
`context`，否则客户端断开就会中途取消；并且要串行化，防止并发点两次。

### 4.5 面板入口

放在**设置菜单**里（顶栏齿轮），而不是新开视图：更新是低频运维动作，
不适合占据一级导航。设置菜单里给一行「版本 vX.Y.Z」+「检测更新」按钮；
检测到新版本时再显示「立即更新」与「回滚」。

流程与现有面板一致：按钮 → `action()` → toast + 操作记录，避免引入新的交互范式。

### 4.6 重启与就绪

`Restart()` 的实现照搬参考实现的思路：

```go
go func() {
    time.Sleep(500 * time.Millisecond) // 先让 HTTP 响应发出去
    os.Exit(0)                         // systemd Restart=always 拉起
}()
```

前提是 unit 里保持 `Restart=always`（现已满足）。Docker 下不应走这条路（见 §7）。

## 5. 安全

- **强制校验和**：不做「没有 checksums 就跳过校验」的降级——本服务进程里持有华为云
  凭证，装进一个未校验的二进制等于交出账号。
- **域名白名单 + HTTPS**：只从 `github.com` / `objects.githubusercontent.com` 等下载。
- **体积上限**：给下载设硬上限（参考实现同样如此）。
- **可选的签名**：校验和本身也可被替换，若重视强度，用 minisign/cosign 签二进制。
- **更新端点必须鉴权**：沿用 `withAuth`，不能做成匿名可触发（否则等于 RCE 入口）。
- **代理**：GitHub 在部分网络需代理；若支持代理配置，凭据只能留在服务端日志之外。

## 6. 与两种部署形态的适配

**systemd（自更新适用）**

- 二进制在 `/opt/codearts2api/bin/`，`ProtectSystem=full` 下可写，替换可行。
- `.backup` 也落在同目录，回滚无需额外权限。
- 新二进制启动失败时，systemd 的 `StartLimitBurst` 会让服务停在 failed 而不无限重启，
  这是好事，但需要文档告诉用户怎么手动回滚。

**Docker（自更新不适用）**

- 容器内替换二进制，下一次 `docker compose up --build` 就被镜像内容覆盖，白做。
- 升级路径：**拉源码后重建镜像**——`git fetch --tags && git checkout <tag> &&
  docker compose up -d --build`。
  注意：本项目**没有**镜像仓库发布链路（`docker-compose.yml` 只有 `build:` 段、
  没有 `image:`；goreleaser 只发 `tar.gz`，不推镜像），所以 `docker compose pull`
  会报找不到镜像，不要写成提示。
- 因此更新端点应能识别「我在容器里」（如存在 `/.dockerenv`）并明确拒绝自更新，
  提示改用上述重建路径，而不是静默失败。

## 7. 边界与取舍

- **只管 server 一个二进制**：面板 `//go:embed` 在 server 里，UI 变更同样需要重启，
  不存在「只热更前端」的场景，所以单文件自更新正好覆盖。`login`/`credit`/`apply` 是
  一次性运维命令，不必在线更新。
- **Windows 不参与**：`os.Exit` 依赖 systemd 拉起；Windows 下无法自替换正在运行的文件。
- **不做静默自动更新**：更新会打断流式请求，必须由人触发。
- **明确不在范围内**：蓝绿/双进程切换、二进制签名校验服务、增量补丁。

## 8. 实施阶段

已按下列三步完成；每一步的验收方式一并记录。

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| 1. 发版基础设施 | 版本变量 + `-version`；`.goreleaser.yaml`；tag 触发 Release workflow；`make` 也注入版本 | 已完成：`go run ./cmd/server -version` 输出 `codearts2api dev (commit none, built unknown)`，正式构建由 ldflags 注入 |
| 2. 更新服务 | `internal/update/` + 4 个端点 | 已完成：`go test ./internal/update/` 10 项全通过；端点层 5 项测试（401 / 未配置 / 版本回显）通过 |
| 3. 面板与文档 | 设置菜单入口 + 操作记录 + README | 已完成：设置菜单「版本」分组含检测更新/立即更新/回滚/重启 |

发版流程：打一个 `v*` tag 即触发 workflow 跑测试并发 Release；本地自测用
`make linux`（版本取自 `git describe`）。首个 Release `v0.1.0` 已按此流程发布成功
（workflow run 35584893591 的 `release` job 52s 通过）。

## 9. 验收与测试要点

- 校验和不匹配时必须**中止且不动现有二进制**（测试里造一个坏 checksum）。
- 平台无匹配 asset 时报错清晰，不静默跳过。
- 替换后旧版存在于 `.backup`，`Rollback()` 能换回并校验其可执行权限。
- 并发调用 `apply` 只执行一次（加锁）。
- 下载中断（客户端断开）不得留下半截临时文件，且不得替换。
- 更新端点未带 key 时返回 401。

## 10. 未决问题

1. 是否需要给下载请求加自定义 UA（部分网络对 GitHub 有拦截）。
2. 是否要给更新请求支持代理配置，以及代理凭据的存放方式。
3. 面板要不要显示 Release 的更新说明全文，还是只显示版本号与链接。
4. 尚未验证「真机点按钮完成一次真实更新」——首个 Release（`v0.1.0`）已就位，
   剩下的前提是一个 Linux + systemd 的部署实例（容器内按设计拒绝自更新）。

## 附：本文核对过的来源

- `Makefile`：4 个构建目标、`-buildvcs=false`、无 ldflags。
- `deploy/codearts2api.service`：`Restart=always`、`RestartSec=5`、`ProtectSystem=full`、
  `ReadWritePaths=/opt/codearts2api/auths /opt/codearts2api/data`。
- `Dockerfile` / `docker-compose.yml`：alpine 运行时、`auths`/`data` 为 volume、
  `config.json` 只读挂载。
- `Wei-Shaw/sub2api`：`backend/internal/service/update_service.go`（imports 与替换顺序）、
  `backend/internal/pkg/sysutil/restart.go`（`os.Exit(0)` 方案）、`.goreleaser.yaml`（ldflags 与
  checksums）、`deploy/install.sh`（从 releases/latest 下载并校验）。
- 仓库发布状态：首个 Release `v0.1.0`（2026-09-21，`hj01857655/codearts2api`），
  asset 为 `codearts2api_linux_{amd64,arm64}.tar.gz` + `checksums.txt`，非 draft、非 prerelease。

参考实现地址：https://github.com/Wei-Shaw/sub2api
