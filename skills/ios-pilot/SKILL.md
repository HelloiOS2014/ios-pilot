---
name: ios-pilot
description: iOS device self-testing workflow for AI agents
trigger: when LLM needs to verify functionality on a real iOS device
---

# iOS Self-Test Workflow

## Prerequisites
- ios-pilot installed (`ios-pilot --help` to verify)
- iOS device connected via USB
- For UI interaction: WDA installed (`ios-pilot wda setup`)

## Workflow

### 1. Confirm device
```bash
ios-pilot device status
```
If not connected: `ios-pilot device connect`

### 2. Build & install (if needed)
```bash
xcodebuild -workspace MyApp.xcworkspace -scheme MyApp -sdk iphoneos -configuration Debug -derivedDataPath ./build
ios-pilot app install ./build/Build/Products/Debug-iphoneos/MyApp.app
ios-pilot app launch com.example.myapp
```

### 3. Observe
```bash
ios-pilot look --annotate
```
Read the element list, identify elements by their numbered labels on the screenshot.

### 4. Interact
```bash
ios-pilot act tap <center_x> <center_y>    # Use center coords from look output
ios-pilot act input "text to type"
ios-pilot act swipe <x1> <y1> <x2> <y2>
```

### 5. Verify
```bash
ios-pilot check screen                      # Screenshot for visual verification
ios-pilot check element --text "Expected"   # Check UI tree (requires WDA)
ios-pilot check app-running com.example.app
ios-pilot check no-crash com.example.app
```

### 6. Debug (on failure)
```bash
ios-pilot log --filter com.example.app
ios-pilot log --level error
ios-pilot log crash
```

### 7. Iterate
Back to step 3. Always `look` before acting.

## Principles
- Always look before acting — never blind-tap
- Use `look --annotate` center coordinates for `act tap`
- `check screen` returns a screenshot — you decide pass/fail visually
- On failure, check logs before concluding it's a code bug
- In degraded mode (no WDA): only screenshots, app management, and logs work
