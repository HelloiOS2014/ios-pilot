# CLAUDE.md — ios-pilot

Go CLI + 自管理 daemon，让 AI agent 通过 6 个命令操作 iOS 真机（device/look/act/app/log/check）。

## 构建与测试

```bash
make build              # 编译二进制
make deploy             # 编译 + 安装到 ~/.local/bin + 安装 Claude Code Skill
make test               # 单元测试
make test-integration   # 集成测试（需要 IOS_DEVICE_CONNECTED=1 + 真机）
make clean              # 清理二进制
```

## 项目结构

```
cmd/ios-pilot/          # 入口（main.go）
internal/
  cli/                  # CLI 命令处理（root.go 路由到子命令）
  client/               # JSON-RPC 客户端
  config/               # 配置加载（所有字段有默认值）
  core/                 # 业务逻辑（DeviceManager, ScreenCapture, UiController, AppManager, LogManager, Checker）
  daemon/               # Daemon 服务器、空闲计时器、PID 文件
  driver/               # 驱动接口定义
    goios/              # go-ios 库实现（设备、应用、截图、日志、tunnel）
    wda/                # WDA HTTP 客户端（W3C WebDriver 协议）
  protocol/             # JSON-RPC 消息类型
skills/ios-pilot/       # Claude Code Skill（SKILL.md）
docs/                   # 架构文档（design.md）
test/integration/       # 真机集成测试
```

## 关键约定

- **日志**：库代码（`internal/driver`, `internal/core`）用 `log/slog`；CLI 命令处理（`internal/cli/`）用 `fmt.Fprintf(os.Stderr, ...)`
- **错误处理**：非致命失败（tunnel、WDA 启动）记录 warning 并降级，不返回 error。只有真正阻止操作的错误才返回
- **iOS 17+ tunnel**：TunnelManager 必须在 daemon 进程内运行（userspace TUN 网络栈是进程本地的）
- **坐标系**：所有坐标为 points（非 pixels）
- **WDA 自动发现**：`findWDABundleID` 遍历已安装应用查找包含 "WebDriverAgent" 的 bundle ID
- **enrichWithTunnel**：连接 RSD 前必须先设置 `UserspaceTUN`/`UserspaceTUNPort`（否则 ConnectTUNDevice 走错路径）
