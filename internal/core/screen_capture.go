package core

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"ios-pilot/internal/config"
	"ios-pilot/internal/driver"
)

// LookResult is the structured outcome of a Look call.
type LookResult struct {
	Screenshot string        `json:"screenshot"`
	Mode       string        `json:"mode"`      // "full" or "degraded"
	ScreenSize [2]int        `json:"screen_size"`
	Elements   []ElementInfo `json:"elements,omitempty"`
}

// ElementInfo describes a single interactive UI element returned by Look.
type ElementInfo struct {
	ID     int    `json:"id"`
	Type   string `json:"type"`
	Label  string `json:"label"`
	Frame  [4]int `json:"frame"`   // [x, y, width, height]
	Center [2]int `json:"center"`  // [centerX, centerY]
}

// ScreenCapture orchestrates screenshot capture and optional annotation.
type ScreenCapture struct {
	screenshotDrv driver.ScreenshotDriver
	wdaDriver     driver.WDADriver
	deviceManager *DeviceManager
	config        *config.Config
}

// NewScreenCapture constructs a ScreenCapture.
func NewScreenCapture(
	sd driver.ScreenshotDriver,
	wd driver.WDADriver,
	dm *DeviceManager,
	cfg *config.Config,
) *ScreenCapture {
	return &ScreenCapture{
		screenshotDrv: sd,
		wdaDriver:     wd,
		deviceManager: dm,
		config:        cfg,
	}
}

// TakeScreenshot takes a screenshot, saves it to disk, and returns the file path.
// It satisfies the ScreenCaptureInterface defined in check.go.
func (sc *ScreenCapture) TakeScreenshot() (string, error) {
	if err := sc.ensureScreenshotDir(); err != nil {
		return "", fmt.Errorf("ensure screenshot dir: %w", err)
	}

	pngBytes, err := sc.captureRaw()
	if err != nil {
		return "", err
	}

	ts := time.Now().UnixNano()
	filename := fmt.Sprintf("screenshot-%d.png", ts)
	path := filepath.Join(sc.config.Screenshot.Dir, filename)

	if err := os.WriteFile(path, pngBytes, 0o644); err != nil {
		return "", fmt.Errorf("write screenshot: %w", err)
	}
	return path, nil
}

// Look captures the screen and optionally annotates it with interactive element overlays.
//
//   - annotate=true: fetch interactive elements, draw numbered bounding boxes, include in result.
//   - ui=true:       include the raw element tree in the result without drawing.
//
// In degraded mode (no WDA), both flags are silently ignored and a plain screenshot is returned.
func (sc *ScreenCapture) Look(annotate bool, ui bool) (*LookResult, error) {
	if err := sc.ensureScreenshotDir(); err != nil {
		return nil, fmt.Errorf("ensure screenshot dir: %w", err)
	}

	pngBytes, err := sc.captureRaw()
	if err != nil {
		return nil, err
	}

	mode := sc.deviceManager.Mode()
	if mode == "" {
		mode = "degraded"
	}

	// Decode to get screen size.
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}
	bounds := img.Bounds()
	screenSize := [2]int{bounds.Dx(), bounds.Dy()}

	ts := time.Now().UnixNano()

	result := &LookResult{
		Mode:       mode,
		ScreenSize: screenSize,
	}

	if mode == "full" && sc.wdaDriver != nil && (annotate || ui) {
		wdaURL := sc.deviceManager.WDAURL()
		sessionID := sc.deviceManager.WDASessionID()

		elements, wdaErr := sc.wdaDriver.GetInteractiveElements(
			wdaURL, sessionID, sc.config.Annotate.InteractiveTypes,
		)
		if wdaErr != nil {
			// Non-fatal: fall back to plain screenshot.
			elements = nil
		}

		// Map to ElementInfo.
		var infos []ElementInfo
		for i, el := range elements {
			cx := el.Frame[0] + el.Frame[2]/2
			cy := el.Frame[1] + el.Frame[3]/2
			infos = append(infos, ElementInfo{
				ID:     i + 1,
				Type:   el.Type,
				Label:  el.Label,
				Frame:  el.Frame,
				Center: [2]int{cx, cy},
			})
		}

		if annotate && len(infos) > 0 {
			annotated, drawErr := sc.drawAnnotations(img, infos)
			if drawErr == nil {
				annotatedFilename := fmt.Sprintf("annotated-%d.png", ts)
				annotatedPath := filepath.Join(sc.config.Screenshot.Dir, annotatedFilename)
				if writeErr := os.WriteFile(annotatedPath, annotated, 0o644); writeErr == nil {
					result.Screenshot = annotatedPath
					result.Elements = infos
					return result, nil
				}
			}
			// If annotation fails, fall through to plain screenshot.
		}

		if ui && len(infos) > 0 {
			result.Elements = infos
		}
	}

	// Save plain screenshot.
	filename := fmt.Sprintf("screenshot-%d.png", ts)
	path := filepath.Join(sc.config.Screenshot.Dir, filename)
	if err := os.WriteFile(path, pngBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write screenshot: %w", err)
	}
	result.Screenshot = path
	return result, nil
}

// captureRaw takes a raw screenshot and returns PNG bytes.
func (sc *ScreenCapture) captureRaw() ([]byte, error) {
	var udid string
	if dev := sc.deviceManager.ConnectedDevice(); dev != nil {
		udid = dev.UDID
	}
	data, err := sc.screenshotDrv.TakeScreenshot(udid)
	if err != nil {
		return nil, fmt.Errorf("take screenshot: %w", err)
	}

	// Accept both raw PNG bytes and base64-encoded PNG.
	if len(data) > 0 && !bytes.HasPrefix(data, []byte("\x89PNG")) {
		decoded, decErr := base64.StdEncoding.DecodeString(string(data))
		if decErr == nil {
			data = decoded
		}
	}
	return data, nil
}

// ensureScreenshotDir creates the screenshot directory if it does not exist.
func (sc *ScreenCapture) ensureScreenshotDir() error {
	return os.MkdirAll(sc.config.Screenshot.Dir, 0o755)
}

// parseHexColor converts a CSS-style hex colour string ("#RRGGBB") to color.RGBA.
// Falls back to red if the string cannot be parsed.
func parseHexColor(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.RGBA{R: 255, A: 255}
	}
	r, rerr := strconv.ParseUint(hex[0:2], 16, 8)
	g, gerr := strconv.ParseUint(hex[2:4], 16, 8)
	b, berr := strconv.ParseUint(hex[4:6], 16, 8)
	if rerr != nil || gerr != nil || berr != nil {
		return color.RGBA{R: 255, A: 255}
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

// drawAnnotations overlays numbered bounding boxes on the provided image and
// returns the result encoded as PNG bytes.
func (sc *ScreenCapture) drawAnnotations(src image.Image, elements []ElementInfo) ([]byte, error) {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	boxColor := parseHexColor(sc.config.Annotate.BoxColor)

	for _, el := range elements {
		x, y, w, h := el.Frame[0], el.Frame[1], el.Frame[2], el.Frame[3]

		// Draw bounding box (2-pixel border).
		drawRect(dst, x, y, w, h, 2, boxColor)

		// Draw label background (small filled square at top-left of bounding box).
		label := fmt.Sprintf("%d", el.ID)
		labelW := len(label)*7 + 4
		labelH := 13
		labelX := x
		labelY := y - labelH
		if labelY < 0 {
			labelY = y
		}

		// Filled background for readability.
		fillRect(dst, labelX, labelY, labelW, labelH, boxColor)

		// Draw label text in white.
		drawLabel(dst, labelX+2, labelY+10, label, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawRect draws a rectangular border of the given thickness on img.
func drawRect(img *image.RGBA, x, y, w, h, thickness int, c color.RGBA) {
	bounds := img.Bounds()
	maxX := bounds.Max.X
	maxY := bounds.Max.Y

	for t := 0; t < thickness; t++ {
		// Top edge.
		for px := x; px < x+w; px++ {
			if px >= 0 && px < maxX && (y+t) >= 0 && (y+t) < maxY {
				img.SetRGBA(px, y+t, c)
			}
		}
		// Bottom edge.
		for px := x; px < x+w; px++ {
			py := y + h - 1 - t
			if px >= 0 && px < maxX && py >= 0 && py < maxY {
				img.SetRGBA(px, py, c)
			}
		}
		// Left edge.
		for py := y; py < y+h; py++ {
			if (x+t) >= 0 && (x+t) < maxX && py >= 0 && py < maxY {
				img.SetRGBA(x+t, py, c)
			}
		}
		// Right edge.
		for py := y; py < y+h; py++ {
			px := x + w - 1 - t
			if px >= 0 && px < maxX && py >= 0 && py < maxY {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

// fillRect fills a rectangle with the given colour.
func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	bounds := img.Bounds()
	maxX := bounds.Max.X
	maxY := bounds.Max.Y
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			if px >= 0 && px < maxX && py >= 0 && py < maxY {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

// drawLabel renders text on img at the given (x, y) pixel using basicfont.Face7x13.
func drawLabel(img *image.RGBA, x, y int, text string, c color.RGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	d.DrawString(text)
}
