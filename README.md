# 明点服务器管理系统

明点服务器管理系统（代号 MDBMC）是一款 Windows 桌面软件，用于在局域网内集中管理多个品牌服务器的 BMC。它通过 **IPMI 2.0（RMCP+）** 和 **Redfish** 两种标准协议统一完成以下工作：

- 设备纳管
- 电源控制
- 读取传感器与事件日志（SEL）
- 采集硬件资产
- 打开 Web 管理页和远程 KVM 控制台

## 功能

| 模块 | 说明 |
| --- | --- |
| 设备管理 | 手动添加，或按网段扫描发现（IPMI + Redfish 探测，自动识别品牌）；支持分组、凭据模板、批量移动分组、批量更换凭据 |
| 电源控制 | 开机、正常关机（ACPI）、强制关机、硬重启、电源循环；支持批量操作，并显示每台设备的执行结果；可控制定位灯 |
| 状态监控 | 后台定时刷新电源与健康状态（间隔可配置），界面实时更新；总览页汇总各类状态与需要关注的设备 |
| 传感器 | 温度、风扇、电压、电源等读数，带阈值刻度与告警级别；可按类型、严重程度筛选并搜索 |
| 事件日志 | 查看、按级别筛选、搜索、分页（每页条数可选）、清空 SEL |
| 硬件资产 | 处理器、内存、磁盘、网卡、电源、固件版本、IPMI FRU；采集结果本地缓存；某一项读取失败时，只在该项右上角显示警告标记，悬停查看原因 |
| Web / KVM | 在独立的 WebView2 窗口中打开 BMC 管理页并自动登录；Supermicro 支持一键打开 HTML5 iKVM；浪潮等使用 Java 远程控制台（JNLP）的 BMC，用随软件安装的 Java 运行环境直接打开，无需本机安装 Java |
| 账号与审计 | 本地账号登录，固定三种角色；登录、电源、控制台、查看密码、配置变更等操作都写入审计日志 |
| 在线更新 | 从 GitHub Releases（[mdbmc-release](https://github.com/12312bg/mdbmc-release)）自动检查新版本，顶栏提示并展示更新说明；管理员一键下载（断点续传）、校验签名后安装并自动重启；支持系统代理、自定义代理和镜像 / 内网更新源，见下方“在线更新” |

## 角色与权限

| 权限 | 管理员 | 操作员 | 只读 |
| --- | :---: | :---: | :---: |
| 查看设备、传感器、事件日志、资产 | ✓ | ✓ | ✓ |
| 打开 BMC Web 登录页 | ✓ | ✓ | ✓ |
| Web 管理自动登录 | ✓ | ✓ | |
| 远程控制台 KVM | ✓ | ✓ | |
| 电源控制、定位灯 | ✓ | ✓ | |
| 清空事件日志 | ✓ | ✓ | |
| 添加 / 扫描 / 编辑 / 删除设备、分组、凭据 | ✓ | ✓ | |
| 查看 BMC 密码明文 | ✓ | ✓ | |
| 用户管理、系统设置、审计日志 | ✓ | | |
| 查看新版本提示和更新说明、手动检查更新 | ✓ | ✓ | ✓ |
| 下载安装软件更新、修改更新设置 | ✓ | | |

权限矩阵集中定义在 `internal/auth/roles.go`。每个后端接口都会单独校验权限，前端只负责隐藏无权限的入口。

## 安全设计

- **登录密码即解锁密码**
  - 所有 BMC 密码用一把随机数据密钥（DEK）加密，算法为 AES-256-GCM。
  - DEK 再用每个账号的登录密码派生出的密钥分别加密保存（Argon2id，64 MiB 内存成本）。
  - 登录成功即解锁；退出或空闲超时自动锁定后，内存中的密钥会被清除。
  - 数据库可以复制到其他电脑，用原账号密码登录即可解密。
  - 管理员可以重置其他用户的密码。**所有管理员都忘记密码时，已保存的 BMC 密码无法恢复。**
- **BMC 证书指纹固定**
  - BMC 通常使用自签名证书，因此采用首次信任（TOFU）：第一次连接时记录 SHA-256 指纹，之后指纹变化即拒绝连接。
  - 需要在设备详情中手动“信任当前证书”才能恢复连接。
  - 控制台窗口同样只接受已固定的证书，见下方“控制台窗口如何连接 BMC”。
- **控制台窗口如何连接 BMC（本地代理）**
  - 老款 BMC 固件常用 TLS 1.0/1.1、旧加密套件或格式不规范的证书，这些都会被 WebView2（Chromium 内核）直接拒绝，窗口打不开页面。
  - 因此控制台窗口不直接连接 BMC：每个窗口进程在本机 `127.0.0.1` 的随机高位端口上启动一个 HTTPS 代理（不用 443 等常见端口，避免被安全软件的 HTTPS 检查拦截），WebView2 只访问这个代理；代理再用与 IPMI/Redfish 相同的 Go TLS 客户端连接 BMC，并且**只接受已固定指纹的 BMC 证书**。
  - 代理使用临时生成的自签名证书，窗口只信任这一张（WebView2 的 `ServerCertificateErrorDetected` 事件，需要 2022 年以后的 WebView2 Runtime，过旧时窗口会提示更新）；页面里直接链接到 BMC 原地址的，同样只放行固定的 BMC 证书。
  - 代理只接受带会话 Cookie 的请求，该 Cookie 只能通过窗口启动时的一次性地址获得，本机其他程序无法借用。
  - 代理会把 BMC 返回的跳转地址、Cookie 的 Domain 改写为代理地址，并把请求的 Host / Origin / Referer 还原为 BMC 地址；WebSocket（HTML5 KVM）同样经过代理。
  - 连接 BMC 期间窗口显示加载页；连接失败、证书不一致等情况显示带原因和“重新连接”按钮的提示页，不会出现白屏。原因同时写入日志。
- **控制台窗口隔离**
  - Web/KVM 页面运行在独立的子进程中（任务管理器里显示为单独的 mdbmc 进程，以及其下的 WebView2 进程），每台设备使用独立的浏览器配置目录。
  - 子进程会监视主程序，主程序退出（包括异常退出）时，所有控制台窗口随之关闭，不会残留进程。
  - 手动退出登录时关闭所有控制台窗口；空闲超时自动锁定时保留已打开的控制台窗口（以免打断用 KVM 安装系统等长时间操作），主界面需重新登录。审计日志分别记为“退出登录”和“空闲锁定”。
  - 自动登录用的密码通过子进程的标准输入传递，不会出现在命令行参数里。
  - 填表脚本只在该 BMC 页面（代理地址）上生效。
  - Java 远程控制台运行的是 BMC 提供的程序（与以前用 Java Web Start 打开时相同），只从固定证书的 BMC 本身下载。
- **初始密码**：管理员新建或重置的密码只能用于登录并修改密码，修改前后端拒绝其他所有操作。
- **登录防爆破**：同一用户名连续 5 次密码错误后，锁定 30 秒。
- **在线更新只安装签名正确的版本**：CI 用 Ed25519 私钥给每个版本的更新清单（含版本号、更新说明和安装包的 SHA-256）签名，软件内置对应公钥。清单签名不符、安装包大小或 SHA-256 与清单不一致的，一律拒绝；安装前会再校验一次下载好的文件。因此发布仓库、镜像站或中间的代理即使被篡改，也无法让软件安装被修改过的程序。详见“在线更新”。
- **权限边界**：所有角色登录后都能解开同一把数据密钥（只读用户也需要连接 BMC 读取状态），所以角色限制只在软件内部生效。拿到数据库文件和任一账号密码的人，可以绕过软件直接解密 BMC 密码。

## 品牌适配

| 品牌 | 状态 |
| --- | --- |
| Supermicro | 通用 IPMI / Redfish 能力全部可用。KVM 优先通过 Redfish OEM `Managers/1/Oem/Supermicro/IKVM` 获取一次性地址直接打开；旧固件回退为自动登录后跳转到经典 HTML5 iKVM 页面 |
| 浪潮 | 通用 IPMI / Redfish 能力可用。KVM：自动登录 Web 后直接启动 Java 远程控制台（AMI JViewer，地址 `/Java/jviewer.jnlp`） |
| Dell / HPE / Lenovo / 华为（超聚变）/ 新华三 / 中科曙光 / 宁畅 / Fujitsu / Cisco / Intel / ASUS / Gigabyte / ASRock Rack / QCT / Tyan / IBM | 自动识别品牌，通用 IPMI / Redfish 能力可用。KVM 暂时打开 Web 管理页并自动登录，在页面里手动启动控制台；如果是 Java 版控制台，点击后会直接打开（见下方） |
| 其他支持 IPMI 2.0 或 Redfish 的 BMC | 按“通用”处理 |

品牌按 BMC 报告的厂商名称（IPMI / Redfish / FRU）识别，识别不到时再按 IPMI 厂商编号（IANA 企业编号，含各品牌子公司和更名后的编号，例如浪潮信息的 IEIT SYSTEMS）识别。界面中的品牌标志来自 [Simple Icons](https://simpleicons.org)（CC0），其中没有的品牌显示为文字标志；各标志的商标权归其所有者。

### Java 远程控制台

不少 BMC（AMI MegaRAC 固件的浪潮、曙光等，以及较老的 Supermicro、Dell、Lenovo）用 Java Web Start（JNLP 文件）启动远程控制台。现在的 Java 已不再提供 Web Start，很多电脑也没装 Java，所以本软件自己运行这类控制台：

- 控制台窗口的本地代理发现 BMC 返回的是 JNLP 文件时，不交给浏览器下载，而是从 BMC 下载其中的程序包（只接受固定证书的 BMC 本身，其他服务器的程序包一律拒绝），解出 Windows 64 位本地库，用**随软件安装的 Java 8 运行环境**（Eclipse Temurin JRE，位于安装目录的 `jre` 文件夹，只供本软件使用）启动。
- 窗口显示“正在启动 Java 远程控制台”页面，实时显示下载进度、已打开或失败原因。程序包按 BMC 缓存，之后再打开会更快。
- Java 控制台随启动它的窗口一起关闭（关闭窗口、退出登录、主程序退出），不会残留进程。
- 老固件的加密 KVM 只支持 TLS 1.0/1.1，因此仅对控制台的 Java 进程重新允许这些协议，不影响系统里的其他 Java。
- JNLP 中的会话令牌按 Java 程序的要求作为命令行参数传给 Java 进程；这些令牌只在本次 BMC 会话内有效。
- Applet 形式的 Java 控制台（如 HP iLO 3/4）暂不支持。

## 技术栈

- **后端**：Go 1.26+
  - [Wails v2](https://wails.io)：桌面壳，使用 WebView2
  - [go-ipmi](https://github.com/bougou/go-ipmi)：IPMI 2.0
  - [gofish](https://github.com/stmcginnis/gofish)：Redfish
  - [modernc SQLite](https://pkg.go.dev/modernc.org/sqlite)：纯 Go 实现，无需 CGO
  - [go-webview2](https://github.com/wailsapp/go-webview2)：控制台子窗口（自建 Win32 窗口 + WebView2）
  - [Eclipse Temurin](https://adoptium.net) 8 JRE：随软件安装，用于运行 BMC 的 Java 远程控制台（GPLv2 + Classpath Exception，许可文件随附在 `jre` 文件夹中）
- **前端**：Vue 3、TypeScript、Vite、[Naive UI](https://www.naiveui.com)、Pinia、Vue Router；支持浅色、深色和跟随系统三种主题

## 项目结构

```
.
├── main.go                       # 入口：Wails 主窗口；带 --console 参数时作为控制台子窗口进程
├── wails.json                    # Wails 项目配置
├── build/                        # 图标、Windows 清单、NSIS 安装包脚本
├── cmd/
│   ├── bmcsim/                   # 模拟 BMC（IPMI + Redfish + Web 登录页），无真机时用于体验和测试
│   ├── devbridge/                # 开发工具：在普通浏览器中运行真实前端 + 真实后端
│   └── release/                  # 发布工具：生成更新签名密钥、生成并签名更新清单（CI 使用）
├── internal/
│   ├── app/                      # Wails 绑定的服务层：权限校验、审计、批量任务、状态轮询、扫描
│   ├── auth/                     # 角色、权限矩阵、登录会话与防爆破
│   ├── vault/                    # 数据密钥的包装（Argon2id）与加解密（AES-GCM）
│   ├── store/                    # SQLite 数据访问与表结构迁移
│   ├── bmc/                      # BMC 驱动：IPMI / Redfish 封装、协议回退、证书固定、厂商识别、网络发现
│   ├── bmcsim/                   # 模拟器实现（测试也使用它）
│   ├── console/                  # 控制台子窗口、本地 HTTPS 代理、加载/错误提示页与自动登录脚本
│   └── update/                   # 在线更新：版本比较、更新清单签名校验、代理、断点续传下载、安装助手
└── frontend/
    ├── src/api/                  # 后端接口封装：wails.ts 调用真实后端，mock.ts 提供浏览器预览用的模拟数据
    ├── src/wailsjs/              # Wails 自动生成的绑定（勿手改）
    ├── src/stores/               # Pinia：会话、设备列表（接收后端实时事件）、主题、软件更新状态
    ├── src/views/                # 页面：总览、设备、设备详情（传感器 / 事件日志 / 资产）、扫描、凭据、用户、审计、设置（含关于与更新）
    ├── src/components/           # 通用组件与图表；UpdateDialog（更新对话框）、MarkdownView（更新说明）
    └── src/layouts/              # 主布局：侧边栏、顶栏（新版本提示）、空闲锁定、强制修改初始密码
```

## 在线更新

软件从 GitHub 仓库 [12312bg/mdbmc-release](https://github.com/12312bg/mdbmc-release) 的 Releases 获取新版本。

- **检查**：启动约 20 秒后检查一次，之后按“系统设置 → 软件更新 → 自动检查更新”的间隔（默认 24 小时，0 为关闭）定期检查；失败时 1 小时后重试。任何用户都可以在“系统设置”中手动检查。
- **提示**：有新版本时，顶栏右侧显示“新版本 vX.Y.Z”，点击打开更新对话框，查看版本、发布日期、更新包大小和更新说明。所有角色都能看到提示，只有管理员能下载和安装。
- **下载**：管理员点击“下载更新”，下载过程中可以关闭对话框在后台继续，顶栏显示进度。网络中断或取消后再次下载会从断点继续。下载完成后校验大小和 SHA-256。
- **安装**：点击“立即安装”并确认后，软件关闭（同时关闭所有 Web / KVM 控制台窗口），由更新助手完成安装并自动重新打开软件。按安装方式不同：
  - **安装版**（用安装包安装，安装目录中有 `uninstall.exe`）：静默运行新版安装包，安装到原目录。安装包需要管理员权限，会弹出 Windows 的 UAC 授权窗口。
  - **免安装版**（解压的 zip）：直接替换 `mdbmc.exe` 和 `jre` 文件夹，替换失败时恢复原文件。程序所在目录需要可写。
- **结果**：重新打开后提示“更新完成”或失败原因（例如 UAC 授权被取消，此时仍是原版本）。安装和结果都记入审计日志（“安装更新”“更新结果”），更新助手的过程写入日志文件，以 `update:` 开头。
- **只接收正式版本**：标签带“-”的版本（如 `v1.2.0-beta.1`）发布为 GitHub 预发布，软件不会收到。

### 网络、代理与更新源

在“系统设置 → 软件更新”中设置（仅管理员）：

- **代理**：留空时使用环境变量 `HTTPS_PROXY` / `HTTP_PROXY`，其次使用 Windows 系统代理（“Internet 选项”中的手动代理及其例外列表，不支持 PAC 自动配置脚本）。也可以填写 `http://主机:端口` 或 `socks5://主机:端口`，只用于检查和下载更新。
- **更新源**：留空即 `https://github.com/12312bg/mdbmc-release/releases`。访问 GitHub 不稳定时，可以填写：
  - 支持 GitHub 地址前缀的镜像（例如 `https://镜像域名/https://github.com/12312bg/mdbmc-release/releases`）；
  - 内网 Web 服务器上的目录，结构与 GitHub Releases 相同：

    ```
    <更新源>/latest/download/latest.json        # 最新版本的更新清单
    <更新源>/latest/download/latest.json.sig    # 清单签名
    <更新源>/download/v1.2.3/mdbmc-amd64-installer.exe
    <更新源>/download/v1.2.3/mdbmc-portable-windows-amd64.zip
    ```

    文件从 GitHub Release 原样下载即可。由于清单有签名，镜像和内网服务器无法篡改更新内容。

## 开发与构建

### 准备环境（Windows）

1. 安装 [Go 1.26+](https://go.dev/dl/) 和 [Node.js 20+](https://nodejs.org/)。
2. 安装 Wails CLI：
   ```powershell
   go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
   ```
3. 确认已安装 WebView2 Runtime。Windows 10/11 一般自带；Windows Server 2016/2019 需要[单独安装](https://developer.microsoft.com/microsoft-edge/webview2/)。
4. 运行 `wails doctor` 检查环境。

### 运行与打包

```powershell
# 开发模式（前端热更新）
wails dev

# 打包单文件 exe，输出到 build\bin\mdbmc.exe
wails build -clean -trimpath

# 生成安装包（需要先安装 NSIS）
wails build -clean -trimpath -nsis
```

在 Linux 或 macOS 上也可以交叉编译出 Windows 版本：

```bash
wails build -platform windows/amd64 -clean -trimpath
```

### 没有真机时体验

```powershell
# 启动 3 台模拟的 Supermicro BMC
# IPMI 端口为 6230-6232/UDP，HTTPS 端口为 8443-8445；账号密码都是 ADMIN / ADMIN
go run ./cmd/bmcsim -count 3
```

在软件中添加设备时，地址填 `127.0.0.1`，并在“高级选项”中填写对应端口。也可以在“扫描发现”页扫描 `127.0.0.1`，同样需要填写端口。

加 `-legacy-tls` 参数可以模拟只支持 TLS 1.0 的老款 BMC 固件（浏览器无法直接打开，用来验证控制台窗口的本地代理）：

```powershell
go run ./cmd/bmcsim -legacy-tls
```

### 前端单独预览

```bash
cd frontend
npm install
npm run dev
```

在浏览器中打开后使用内置模拟数据：用户名为 admin、zhangwei 或 liuyang，密码填任意 8 位以上即可。

如果要在浏览器中连接真实后端（例如在 Linux 上调试）：

```bash
cd frontend && npm run build && cd ..
go run ./cmd/devbridge -sim 2      # 然后打开 http://127.0.0.1:34115
```

### 发布新版本

#### 一次性准备

1. **生成更新签名密钥**（在自己的电脑上执行）：
   ```bash
   go run ./cmd/release genkey -out mdbmc-update.key
   ```
   命令输出公钥，私钥写入 `mdbmc-update.key`。
2. 在 mdbmc 仓库 **Settings → Secrets and variables → Actions** 中添加：
   - **Variables** → `UPDATE_PUBLIC_KEY`：上一步输出的公钥。编译时写入程序，用于校验更新。
   - **Secrets** → `UPDATE_PRIVATE_KEY`：`mdbmc-update.key` 文件的内容。之后把该文件离线妥善保存，不要提交到仓库。**私钥丢失后，已安装的版本将无法再在线更新**（只能手动安装带新公钥的版本）；私钥泄露时应尽快换新密钥。
   - **Secrets** → `RELEASE_TOKEN`：一个 [Fine-grained personal access token](https://github.com/settings/personal-access-tokens/new)，Repository access 只选 `12312bg/mdbmc-release`，权限 **Contents: Read and write**。用于在 mdbmc-release 创建 Release。
3. mdbmc-release 仓库需要至少有一个提交（已有 README），Release 的标签建在它的默认分支上。

换密钥时，可以让 `UPDATE_PUBLIC_KEY` 暂时同时包含新旧两个公钥（用英文逗号分隔），先发布一个用旧私钥签名、内置两个公钥的版本，再改用新私钥。

#### 发布

更新说明取自附注标签（annotated tag）的内容，支持 Markdown；用轻量标签或标签内容为空时，自动列出与上一个标签之间的提交。

```bash
# notes.md 写本次的更新说明；--cleanup=verbatim 保留以 # 开头的 Markdown 标题
git tag -a v1.2.0 -F notes.md --cleanup=verbatim
git push origin v1.2.0
```

CI 编译后，`release` 作业会：

- 生成 `latest.json`（版本、更新说明、各安装包的大小和 SHA-256）并用私钥签名得到 `latest.json.sig`，同时用 `UPDATE_PUBLIC_KEY` 验证签名，密钥不匹配时停止发布；
- 在 mdbmc-release 创建 Release `v1.2.0`，上传安装包、免安装版 zip、`latest.json`、`latest.json.sig` 和 `SHA256SUMS.txt`。

重新运行失败的作业时，会覆盖同名 Release 中的文件。标签带“-”（如 `v1.2.0-beta.1`）时发布为预发布版本，软件的在线更新不会收到。

本地调试在线更新：没有内置公钥的开发版本可以用环境变量 `MDBMC_UPDATE_PUBLIC_KEY` 指定公钥（正式版本忽略该变量），并在“系统设置”中把更新源指向本地服务器。

### 测试

```bash
go test ./...                       # 单元测试 + 基于模拟器的 IPMI / Redfish / 服务层集成测试
cd frontend && npm run typecheck    # 前端类型检查
```

### 持续集成（GitHub Actions）

每次推送都会运行 `.github/workflows/build.yml`：

- **Linux**：前端类型检查与构建、`go vet`（含 Windows 目标）、`go test -race`（含在线更新的签名校验、断点续传、篡改检测、免安装版替换测试）。
- **Windows**：
  - 先跑控制台窗口冒烟测试：打开真实的 WebView2 窗口访问一个只支持 TLS 1.0 的模拟 BMC（WebView2 无法直连），确认页面能经本地代理打开并自动填写、提交登录表单，且 BMC 证书与固定值不一致时拒绝连接。
  - 测试前先下载固定版本的 Temurin 8 JRE 并校验 SHA-256（版本和校验值见工作流中的 `JRE_URL`、`JRE_SHA256`），冒烟测试会用它经 JNLP 启动一个测试用 Java 程序，并确认关闭窗口后 Java 进程随之结束。
  - 再编译 `mdbmc.exe`、NSIS 安装包（含 Java 运行环境）和 `bmcsim.exe`，并打包免安装版 `mdbmc-portable-windows-amd64.zip`（`mdbmc.exe` 与 `jre` 文件夹）。

编译产物在 Actions 运行页面的 **Artifacts** 中下载。单独的 `mdbmc.exe` 不带 Java 运行环境，需要 Java 远程控制台时请用安装包或免安装版 zip。推送 `v*` 标签时，版本号取自标签（同时写入安装包的版本信息）；其他构建的版本号是“下一个补丁版本-dev.距上个标签的提交数+提交号”，例如 v1.2.3 之后第 5 个提交为 `1.2.4-dev.5+abc1234`，在线更新把它看作比 v1.2.3 新、比 v1.2.4 旧，并由 **release** 作业发布到 mdbmc-release，见上方“发布新版本”。编译时通过 `-ldflags` 写入版本号和更新签名公钥（仓库变量 `UPDATE_PUBLIC_KEY`）。

本地开发时，可以用环境变量 `MDBMC_JAVA_HOME` 指定 Java 运行环境目录（其中有 `bin\javaw.exe`），以代替安装目录中的 `jre`。

## 数据存放位置

- 数据库：`%APPDATA%\mdbmc\mdbmc.db`
- 日志：`%APPDATA%\mdbmc\mdbmc.log`
- 控制台窗口的浏览器配置：`%APPDATA%\mdbmc\console\<设备ID>\`
- Java 远程控制台从 BMC 下载的程序包缓存：`%APPDATA%\mdbmc\console\<设备ID>\java\`
- 在线更新下载的安装包和更新助手：`%APPDATA%\mdbmc\updates\`（安装成功后自动清理）

可以通过环境变量 `MDBMC_DATA_DIR` 指定其他目录。

## 故障排查

- **Web 管理 / 远程控制台窗口打不开页面**：窗口内的提示页会说明原因（连接超时、BMC 拒绝连接、TLS 握手失败、证书与已信任的不一致等），展开“技术详情”可看到具体错误。
- 每个控制台窗口的过程都会写入日志 `%APPDATA%\mdbmc\mdbmc.log`，以 `console window "<设备名> - Web 管理"` 开头，例如：
  - `proxy https://127.0.0.1:<端口> -> https://<BMC 地址>`：本地代理已启动；
  - `certificate-allowed .../__mdbmc/start`：窗口已连上本地代理；
  - `proxy-request GET / -> 200 (350 ms)`：代理向 BMC 请求页面的结果；
  - `loaded /`、`submitted /`：页面已加载、已自动提交登录表单；
  - `proxy-error ...`：代理连接 BMC 失败的原因；`proxy-log ...`：BMC 返回内容中途出错；`proxy-server ...`：窗口与本地代理之间的连接错误；
  - `navigation-failed ...`、`certificate-rejected ...`、`process-failed ...`：WebView2 层面的错误；
  - `autologin ...`：自动登录找到了哪些输入框、点了哪个按钮，或卡在哪一步；
  - `java-preparing ...`、`java-running`、`java-exited ...`、`java-failed ...`：Java 远程控制台的下载、启动与退出（含 Java 输出的最后几行）。
- 在线更新的安装过程以 `update:` 开头，例如 `update: installing 1.2.0 over 1.1.0 (installer, ...)`、`update: installed 1.2.0` 或 `update: failed: ...`。检查更新失败时，“系统设置”的“关于”卡片中显示原因（无法连接、签名校验失败等）。
- 反馈问题时请附上这些日志行（不含密码）。
