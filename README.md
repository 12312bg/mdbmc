# MDBMC · 多品牌服务器带外管理

MDBMC 是一款 Windows 桌面软件，用于在局域网内集中管理多个品牌服务器的 BMC。它通过 **IPMI 2.0（RMCP+）** 和 **Redfish** 两种标准协议统一完成以下工作：

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
| 传感器 | 温度、风扇、电压、电源等读数，带阈值刻度与告警级别 |
| 事件日志 | 查看、筛选、清空 SEL |
| 硬件资产 | 处理器、内存、磁盘、网卡、电源、固件版本、IPMI FRU；采集结果本地缓存 |
| Web / KVM | 在独立的 WebView2 窗口中打开 BMC 管理页并自动登录；Supermicro 支持一键打开 HTML5 iKVM |
| 账号与审计 | 本地账号登录，固定三种角色；登录、电源、控制台、查看密码、配置变更等操作都写入审计日志 |

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
  - 打开控制台窗口前也会先校验指纹。
- **控制台窗口隔离**
  - Web/KVM 页面运行在独立的子进程中，每台设备使用独立的浏览器配置目录。
  - 自动登录用的密码通过子进程的标准输入传递，不会出现在命令行参数里。
  - 填表脚本只在该 BMC 的源地址上生效。
- **登录防爆破**：同一用户名连续 5 次密码错误后，锁定 30 秒。

## 品牌适配

| 品牌 | 状态 |
| --- | --- |
| Supermicro | 通用 IPMI / Redfish 能力全部可用。KVM 优先通过 Redfish OEM `Managers/1/Oem/Supermicro/IKVM` 获取一次性地址直接打开；旧固件回退为自动登录后跳转到经典 HTML5 iKVM 页面 |
| Dell / HPE / Lenovo / 华为 / 浪潮 / 新华三 | 自动识别品牌，通用 IPMI / Redfish 能力可用。KVM 暂时打开 Web 管理页并自动登录，在页面里手动启动控制台 |
| 其他支持 IPMI 2.0 或 Redfish 的 BMC | 按“通用”处理 |

Java 版 KVM（老旧机型）暂不支持。

## 技术栈

- **后端**：Go 1.26+
  - [Wails v2](https://wails.io)：桌面壳，使用 WebView2
  - [go-ipmi](https://github.com/bougou/go-ipmi)：IPMI 2.0
  - [gofish](https://github.com/stmcginnis/gofish)：Redfish
  - [modernc SQLite](https://pkg.go.dev/modernc.org/sqlite)：纯 Go 实现，无需 CGO
  - [go-webview2](https://github.com/jchv/go-webview2)：控制台子窗口
- **前端**：Vue 3、TypeScript、Vite、[Naive UI](https://www.naiveui.com)、Pinia、Vue Router；支持浅色、深色和跟随系统三种主题

## 项目结构

```
.
├── main.go                       # 入口：Wails 主窗口；带 --console 参数时作为控制台子窗口进程
├── wails.json                    # Wails 项目配置
├── build/                        # 图标、Windows 清单、NSIS 安装包脚本
├── cmd/
│   ├── bmcsim/                   # 模拟 BMC（IPMI + Redfish + Web 登录页），无真机时用于体验和测试
│   └── devbridge/                # 开发工具：在普通浏览器中运行真实前端 + 真实后端
├── internal/
│   ├── app/                      # Wails 绑定的服务层：权限校验、审计、批量任务、状态轮询、扫描
│   ├── auth/                     # 角色、权限矩阵、登录会话与防爆破
│   ├── vault/                    # 数据密钥的包装（Argon2id）与加解密（AES-GCM）
│   ├── store/                    # SQLite 数据访问与表结构迁移
│   ├── bmc/                      # BMC 驱动：IPMI / Redfish 封装、协议回退、证书固定、厂商识别、网络发现
│   ├── bmcsim/                   # 模拟器实现（测试也使用它）
│   └── console/                  # 控制台子窗口与自动登录脚本
└── frontend/
    ├── src/api/                  # 后端接口封装：wails.ts 调用真实后端，mock.ts 提供浏览器预览用的模拟数据
    ├── src/wailsjs/              # Wails 自动生成的绑定（勿手改）
    ├── src/stores/               # Pinia：会话、设备列表（接收后端实时事件）、主题
    ├── src/views/                # 页面：总览、设备、设备详情（传感器 / 事件日志 / 资产）、扫描、凭据、用户、审计、设置
    ├── src/components/           # 通用组件与图表
    └── src/layouts/              # 主布局：侧边栏、顶栏、空闲锁定、强制修改初始密码
```

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

### 测试

```bash
go test ./...                       # 单元测试 + 基于模拟器的 IPMI / Redfish / 服务层集成测试
cd frontend && npm run typecheck    # 前端类型检查
```

## 数据存放位置

- 数据库：`%APPDATA%\mdbmc\mdbmc.db`
- 日志：`%APPDATA%\mdbmc\mdbmc.log`
- 控制台窗口的浏览器配置：`%APPDATA%\mdbmc\console\<设备ID>\`

可以通过环境变量 `MDBMC_DATA_DIR` 指定其他目录。
