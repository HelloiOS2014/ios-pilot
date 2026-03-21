---
name: ios-pilot
description: iOS device automation workflow for AI agents
trigger: when LLM needs to interact with, test, or automate a real iOS device
---

# iOS Device Automation Workflow

## Prerequisites
- ios-pilot installed (`ios-pilot --help` to verify)
- iOS device connected via USB
- For UI interaction: WDA installed on device (`ios-pilot wda setup`)
  - Only needs to be installed once; `device connect` auto-launches WDA

## Workflow

### 1. Confirm device
```bash
ios-pilot device status
```
If not connected: `ios-pilot device connect`

Check the response for `wda.mode`:
- `"full"` — all commands available (tap, swipe, input, element inspection)
- `"degraded"` — screenshots, app management, and logs only

### 2. Build & install (if needed)
```bash
xcodebuild -workspace MyApp.xcworkspace -scheme MyApp -sdk iphoneos -configuration Debug -derivedDataPath ./build
ios-pilot app install ./build/Build/Products/Debug-iphoneos/MyApp.app
ios-pilot app launch com.example.myapp
```

### 3. Observe
```bash
ios-pilot look --annotate    # Screenshot with numbered element overlays
ios-pilot look --ui          # Screenshot + raw element JSON (for programmatic use)
ios-pilot look               # Screenshot only
```

`look --ui` returns JSON:
```json
{
  "screenshot": "/path/to/screenshot.png",
  "screen_size": [390, 844],
  "elements": [
    {"id": 1, "type": "button", "label": "Login", "frame": [150, 400, 90, 44], "center": [195, 422]},
    {"id": 2, "type": "text", "label": "Welcome", "frame": [50, 100, 290, 30], "center": [195, 115]}
  ]
}
```

- Use `--annotate` when you need a visual overview of the screen
- Use `--ui` when you need to search elements programmatically (by label, type, or position)

### 4. Interact
```bash
ios-pilot act tap <x> <y>                      # Tap at point coordinates
ios-pilot act swipe <x1> <y1> <x2> <y2>        # Swipe between two points
ios-pilot act input "<text>"                    # Type into focused field
ios-pilot act press home|volumeUp|volumeDown|lock  # Hardware buttons
```

### 5. Verify
```bash
ios-pilot check screen                      # Screenshot for visual verification
ios-pilot check element --text "Expected"   # Check element exists in UI tree
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

## Coordinate System

- All coordinates (`tap`, `swipe`, element `center`/`frame`) are in **points**, not pixels
- `screen_size` from `look` returns `[width, height]` in points
- On Retina displays: pixels = points × scale factor (2x or 3x), but you always use points
- `swipe` takes start point `(x1, y1)` and end point `(x2, y2)`, not deltas

## Common Patterns

### Scroll
```bash
# Scroll content DOWN (swipe up): from 75% to 25% of screen height
ios-pilot act swipe <midX> <h*75/100> <midX> <h*25/100>

# Scroll content UP (swipe down): from 25% to 75%
ios-pilot act swipe <midX> <h*25/100> <midX> <h*75/100>
```
Where `midX = screen_width / 2` and `h = screen_height` (from `screen_size`). Wait 2 seconds after each scroll for animation.

### Navigate back
1. Use `look --ui`, find an element with label containing "返回" or "Back", tap its center
2. Fallback — iOS edge swipe gesture:
```bash
ios-pilot act swipe 5 <midY> <w/3> <midY>
```

### App lifecycle
```bash
ios-pilot app launch <bundle_id>    # Start or bring to foreground
ios-pilot app kill <bundle_id>      # Force quit
ios-pilot app foreground            # Get current foreground app's bundle ID
ios-pilot app list                  # List all installed apps (with bundle IDs)
```

### Find elements
Use `look --ui` JSON output to locate elements:
- **By exact label**: find element where `label == "Submit"`
- **By partial label**: find element where label contains `"热榜"`
- **By type**: filter by `type` field — `"button"`, `"text"`, `"textfield"`, `"cell"`, `"link"`, `"switch"`, `"icon"`
- **By position**: use `center` coordinates to identify elements in specific screen regions
- Then tap the element's `center` coordinates: `ios-pilot act tap <cx> <cy>`

## Principles
- Always `look` before acting — never blind-tap
- Use coordinates from `look --ui` or `look --annotate` for `act tap`
- `check screen` returns a screenshot — you decide pass/fail visually
- On failure, check logs before concluding it's a code bug
- In degraded mode (no WDA): only screenshots, app management, and logs work
