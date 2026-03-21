# ios-pilot

Go CLI + 自管理 daemon，让 AI agent 操作 iOS 真机进行开发自测和设备操控，基于 [go-ios](https://github.com/danielpaulus/go-ios) 库 + [WebDriverAgent](https://github.com/appium/WebDriverAgent)。

## 文档

- [设计规格](docs/design.md)
- [Skill（iOS 自测工作流）](skills/ios-pilot/SKILL.md)

## 功能特性

- **6+2 命令体系**：6 个主要命令（device/look/act/app/log/check）+ 2 个管理命令（wda/daemon）
- **设备管理**：列出、连接、断开 iOS 设备
- **应用管理**：安装、启动、终止、卸载应用
- **UI 自动化**：点击、滑动、输入、按键（基于 WDA W3C WebDriver 协议）
- **截图与标注**：截图 + 可选 UI 元素编号标注，返回元素坐标供 AI 使用
- **日志与崩溃**：syslog 日志捕获、崩溃报告查看
- **断言验证**：屏幕截图、元素存在、应用运行、无崩溃检查
- **降级模式**：WDA 不可用时自动降级，仍支持截图、应用管理、日志等核心功能
- **自管理 daemon**：首次调用自动启动，空闲 30 分钟自动退出

## 前置条件

1. **macOS 系统**

2. **Go 环境**（用于编译）
   ```bash
   go version  # 需要 Go 1.25+
   ```

3. **go-ios**（已集成为 Go 库依赖，编译时自动拉取，无需单独安装）

4. **iOS 设备准备**
   - 通过 USB 连接 iOS 设备
   - 信任此电脑

5. **WebDriverAgent**（可选，UI 自动化需要）
   - 在设备上安装并签名 WDA（只需安装一次，无需保持 xcodebuild 运行）
   - 安装指南：`ios-pilot wda setup`
   - `ios-pilot device connect` 会自动启动 WDA 进程、端口转发、创建 session

> iOS 17+ 的 tunnel 由 daemon 自动管理，无需手动运行 `sudo pymobiledevice3 remote tunneld`。

## 安装

**一键安装（无需 Go 环境）：**
```bash
curl -sSL https://raw.githubusercontent.com/HelloiOS2014/ios-pilot/main/install.sh | bash
```

**或在仓库内运行：**
```bash
./install.sh
```

安装脚本会：
1. 从 GitHub Releases 下载预编译二进制（无 Release 时 fallback 到本地编译）
2. 安装到 `~/.local/bin/ios-pilot`
3. 交互式选择 Claude Code Skill 安装位置（全局 / 指定项目 / 跳过）

非交互模式：
```bash
INSTALL_SKILL=global ./install.sh            # Skill 装全局
INSTALL_SKILL=/path/to/project ./install.sh  # 装到指定项目
VERSION=v0.2.0 ./install.sh                  # 指定版本
```

验证安装：
```bash
ios-pilot --version
```

## 快速开始

```bash
# 1. 查看可用设备
ios-pilot device list

# 2. 连接设备（单设备时自动选择）
ios-pilot device connect

# 3. 截图
ios-pilot look

# 4. 截图 + UI 元素标注（需要 WDA）
ios-pilot look --annotate

# 5. 点击标注中的元素坐标
ios-pilot act tap 200 400

# 6. 输入文本
ios-pilot act input "Hello"

# 7. 查看日志
ios-pilot log -n 20 --filter com.example.app

# 8. 查看崩溃
ios-pilot log crash

# 9. 断言验证
ios-pilot check app-running com.example.app
ios-pilot check no-crash com.example.app

# 10. 断开连接
ios-pilot device disconnect
```

## 命令参考

### device — 设备管理

| 子命令 | 说明 |
|--------|------|
| `device list` | 列出所有连接的 iOS 设备 |
| `device connect [udid]` | 连接设备（省略 UDID 则自动选择） |
| `device status` | 查看连接状态和 WDA 信息 |
| `device disconnect` | 断开连接 |

### look — 观察设备状态

| 用法 | 说明 |
|------|------|
| `look` | 仅截图 |
| `look --ui` | 截图 + UI 元素树（JSON） |
| `look --annotate` | 截图 + 编号元素标注（需要 WDA） |

`--annotate` 返回每个交互元素的编号、类型、标签和中心坐标，供 AI 精确定位点击位置。

### act — UI 操作

| 用法 | 说明 |
|------|------|
| `act tap <x> <y>` | 点击坐标（需要 WDA） |
| `act swipe <x1> <y1> <x2> <y2>` | 滑动（需要 WDA） |
| `act input "<text>"` | 向当前焦点输入文本（需要 WDA） |
| `act press <key>` | 按键：home / volumeUp / volumeDown / lock |

### app — 应用管理

| 子命令 | 说明 |
|--------|------|
| `app list` | 列出已安装应用 |
| `app install <path>` | 安装 .app 或 .ipa |
| `app launch <bundle_id>` | 启动应用 |
| `app kill <bundle_id>` | 终止应用 |
| `app uninstall <bundle_id>` | 卸载应用 |
| `app foreground` | 获取前台应用 bundle ID |

### log — 日志与崩溃

| 用法 | 说明 |
|------|------|
| `log` | 最近 50 条日志 |
| `log -n <count>` | 指定条数 |
| `log --filter <name>` | 按进程/bundle 过滤 |
| `log --level <level>` | 按日志级别过滤 |
| `log --search <text>` | 搜索日志内容 |
| `log --follow` | 持续跟踪新日志 |
| `log crash` | 列出崩溃报告 |
| `log crash <id>` | 查看指定崩溃详情 |

### check — 断言验证

| 子命令 | 说明 |
|--------|------|
| `check screen` | 截图供 LLM 视觉验证 |
| `check element --text "<text>"` | 检查 UI 元素是否存在（需要 WDA） |
| `check app-running <bundle_id>` | 检查应用是否在前台 |
| `check no-crash <bundle_id>` | 验证应用无崩溃 |

### wda — WebDriverAgent 管理

| 子命令 | 说明 |
|--------|------|
| `wda setup` | 显示 WDA 安装指南 |
| `wda status` | 检查 WDA 运行状态 |
| `wda restart` | 重启 WDA 会话 |

### daemon — Daemon 管理

| 子命令 | 说明 |
|--------|------|
| `daemon status` | 显示 daemon PID、运行时间、设备信息 |
| `daemon stop` | 优雅关闭 daemon |

## 架构

```
┌──────────────┐         Unix Socket          ┌──────────────────┐
│   CLI 命令    │  ── JSON-RPC 2.0 ──────────▶ │     Daemon       │
│  (ios-pilot)  │                              │                  │
└──────────────┘                              │  ┌────────────┐  │
                                              │  │DeviceManager│  │
首次调用自动 fork daemon                        │  │ScreenCapture│  │
空闲 30min 自动退出                             │  │UiController │  │
                                              │  │AppManager   │  │
                                              │  │LogManager   │  │
                                              │  │Checker      │  │
                                              │  └──────┬─────┘  │
                                              └─────────┼────────┘
                                                        │
                                              ┌─────────┴────────┐
                                              │   Driver 层       │
                                              │                   │
                                              │  goios/  ← go-ios 库（设备、应用、截图、日志）
                                              │  wda/   ← WDA HTTP（W3C WebDriver 协议）
                                              └───────────────────┘
```

- **单二进制**：CLI 和 daemon 共用同一个 Go 二进制
- **通信协议**：JSON-RPC 2.0 over Unix socket (`~/.config/ios-pilot/pilot.sock`)
- **go-ios 库调用**：直接使用 Go 库获取结构化数据，不依赖 CLI + 正则解析
- **WDA 通信**：标准 W3C WebDriver 协议，HTTP 请求
- **iOS 17+ tunnel**：daemon 自动管理，透明处理

## 配置

配置文件：`~/.config/ios-pilot/config.json`（可选，所有字段都有默认值）

```json
{
  "idle_timeout": "30m",
  "log_buffer_size": 2000,
  "wda": {
    "auto_start": true,
    "bundle_id": "com.facebook.WebDriverAgentRunner.xctrunner",
    "health_interval": "30s",
    "max_restart": 3
  },
  "screenshot": {
    "dir": "~/.config/ios-pilot/screenshots",
    "retention_hours": 24,
    "max_count": 200
  },
  "annotate": {
    "box_color": "#FF0000",
    "label_size": 14,
    "interactive_types": ["button", "textfield", "switch", "link", "cell", "icon", "text"]
  }
}
```

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `idle_timeout` | `"30m"` | Daemon 空闲超时时间 |
| `log_buffer_size` | `2000` | 日志环形缓冲区大小 |
| `wda.auto_start` | `true` | 连接设备时自动启动 WDA 会话 |
| `wda.bundle_id` | `"com.facebook.WebDriverAgentRunner.xctrunner"` | WDA 的 bundle ID |
| `wda.health_interval` | `"30s"` | WDA 健康检查间隔 |
| `wda.max_restart` | `3` | WDA 最大自动重启次数 |
| `screenshot.dir` | `"~/.config/ios-pilot/screenshots"` | 截图存储目录 |
| `screenshot.retention_hours` | `24` | 截图保留时间（小时） |
| `screenshot.max_count` | `200` | 最大截图数量 |
| `annotate.box_color` | `"#FF0000"` | 标注框颜色 |
| `annotate.label_size` | `14` | 标注标签字号 |
| `annotate.interactive_types` | `["button", "textfield", ...]` | 参与标注的元素类型 |

## 降级模式

当 WDA 不可用时（未安装或未运行），ios-pilot 自动进入降级模式。

### 可用功能

| 命令 | 降级模式下可用 |
|------|---------------|
| `device` 全部子命令 | ✅ |
| `look`（仅截图） | ✅ |
| `look --annotate` / `look --ui` | ❌ 需要 WDA |
| `act press`（硬件按键） | ✅ |
| `act tap` / `act swipe` / `act input` | ❌ 需要 WDA |
| `app` 全部子命令 | ✅ |
| `log` 全部功能 | ✅ |
| `check screen` | ✅ |
| `check element` | ❌ 需要 WDA |
| `check app-running` | ✅ |
| `check no-crash` | ✅ |

通过 `ios-pilot device status` 可查看当前模式（`full` 或 `degraded`）。

## License

MIT
