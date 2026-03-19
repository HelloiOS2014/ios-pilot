# 集成测试已知问题

## 1. WDA element tree 被 InteractiveTypes 过滤为空

**现象**: `look --annotate` 和 `look --ui` 返回 0 个 elements，尽管 WDA mode=full。

**根因**: WDA source tree 返回的主屏幕元素类型主要是 `XCUIElementTypeOther`、`XCUIElementTypeIcon`、`XCUIElementTypeWindow`，
经过 `mapTypeName()` 映射后变成 `other`、`icon`、`window`，不在默认的 `InteractiveTypes`
配置 `["button", "textfield", "switch", "link", "cell"]` 中，被 `FilterInteractive()` 全部过滤掉了。

**验证**: 手动 curl `GET /session/{id}/source` 返回了完整的 XML 树（82KB），元素本身是有的，只是类型不匹配。

**修复方向**: `InteractiveTypes` 默认列表需要加上 `icon`、`cell`、`text` 等常见类型，或者在主屏幕场景下不做过滤。

## 2. WDA HTTP 响应状态码未检查（已修复）

**现象**: `act tap/swipe/press` 永远返回 `{"status": "ok"}`，即使 WDA 返回 4xx/5xx。

**根因**: `postActions()`、`PressButton()`、`InputText()` 只检查了 `http.Post()` 的网络错误，
没有检查 HTTP 响应状态码。WDA 返回错误时被静默忽略。

**修复**: 已在 `internal/driver/wda/client.go` 添加 `checkResponse()` 方法，所有写操作现在都会检查 HTTP 2xx。

## 3. App Launch/Kill/Foreground 需要 Developer Image

**现象**: `app launch com.apple.Preferences` 报错 `InvalidService: Could not start service:com.apple.instruments.remoteserver.DVTSecureSocketProxy`。

**根因**: instruments 服务需要挂载 Developer Image。iOS 17+ 设备走 appservice 路径不需要，
但老设备（如 iPhone 11）回退到 instruments 路径时失败。

**影响**: 测试 06-08、16-17 会被 skip。

**修复方向**: 考虑通过 WDA 的 session capabilities 启动 app（`POST /session` 时指定 `bundleId`），
绕过 instruments 依赖。

## 4. 测试坐标打空气

**现象**: `act tap 200 400` 在主屏幕上没有可见效果，截图前后一致。

**根因**: (200, 400) 在 iPhone 11 (414x896pt) 主屏幕上落在图标之间的空白区域。
同理 swipe down 的坐标也可能没有触发通知中心。

**验证**: `act swipe 200 200 200 600`（手指向下滑）和 `act press home` 确实改变了屏幕，
说明 WDA 操作链路本身是通的。

**修复方向**: 测试中先用 `look --ui` 获取实际元素坐标，再用返回的 center 坐标做 tap/swipe，
而不是硬编码坐标。这依赖问题 1 先修复。

## 5. `app list` 只返回用户 App

**现象**: `app list` 返回 14 个 app，不包含 `com.apple.Preferences` 等系统 app。

**根因**: `GoIosAppDriver.ListApps()` 调用 `installationproxy.BrowseUserApps()`，只返回用户安装的 app。

**影响**: 测试中无法通过 `app list` 验证系统 app 的存在。已调整断言只检查返回结构正确性。
