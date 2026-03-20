// Package wda provides a W3C WebDriver HTTP client for WebDriverAgent (WDA).
// It implements the driver.WDADriver interface using standard net/http.
package wda

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ios-pilot/internal/driver"
)

// Compile-time interface check.
var _ driver.WDADriver = (*WDAClient)(nil)

// WDAClient is an HTTP client for WebDriverAgent.
// All touch actions use the W3C Actions API (WDA 7.0+).
type WDAClient struct {
	http *http.Client
}

// NewWDAClient creates a new WDAClient with a default HTTP timeout.
func NewWDAClient() *WDAClient {
	return &WDAClient{
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Status checks whether WDA is ready by calling GET {wdaURL}/status.
// Returns true when {"value":{"ready":true}} is received.
func (c *WDAClient) Status(wdaURL string) (bool, error) {
	resp, err := c.http.Get(wdaURL + "/status")
	if err != nil {
		return false, fmt.Errorf("wda status: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value struct {
			Ready bool `json:"ready"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("wda status: decode response: %w", err)
	}
	return result.Value.Ready, nil
}

// CreateSession creates a new WDA session and returns the session ID.
// It calls POST {wdaURL}/session with an empty capabilities object.
func (c *WDAClient) CreateSession(wdaURL string) (string, error) {
	body := map[string]interface{}{
		"capabilities": map[string]interface{}{},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("create session: marshal body: %w", err)
	}

	resp, err := c.http.Post(wdaURL+"/session", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value struct {
			SessionID string `json:"sessionId"`
		} `json:"value"`
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("create session: decode response: %w", err)
	}

	// Some WDA versions return sessionId at the top level, others inside value.
	sessionID := result.Value.SessionID
	if sessionID == "" {
		sessionID = result.SessionID
	}
	if sessionID == "" {
		return "", fmt.Errorf("create session: empty session ID in response")
	}
	return sessionID, nil
}

// DeleteSession terminates the WDA session by calling DELETE {wdaURL}/session/{sessionID}.
func (c *WDAClient) DeleteSession(wdaURL string, sessionID string) error {
	req, err := http.NewRequest(http.MethodDelete, wdaURL+"/session/"+sessionID, nil)
	if err != nil {
		return fmt.Errorf("delete session: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	resp.Body.Close()
	return nil
}

// Tap performs a tap gesture at (x, y) using the W3C Actions API.
func (c *WDAClient) Tap(wdaURL string, sessionID string, x, y int) error {
	actions := buildPointerActions("finger1", []pointerAction{
		{Type: "pointerMove", Duration: 0, X: x, Y: y},
		{Type: "pointerDown", Button: 0},
		{Type: "pause", Duration: 100},
		{Type: "pointerUp", Button: 0},
	})
	return c.postActions(wdaURL, sessionID, actions)
}

// Swipe performs a swipe gesture from (x1, y1) to (x2, y2) using the W3C Actions API.
func (c *WDAClient) Swipe(wdaURL string, sessionID string, x1, y1, x2, y2 int) error {
	actions := buildPointerActions("finger1", []pointerAction{
		{Type: "pointerMove", Duration: 0, X: x1, Y: y1},
		{Type: "pointerDown", Button: 0},
		{Type: "pointerMove", Duration: 800, X: x2, Y: y2},
		{Type: "pointerUp", Button: 0},
	})
	return c.postActions(wdaURL, sessionID, actions)
}

// InputText sends the given text to the current focused element by splitting
// it into individual characters per the WDA /wda/keys API.
func (c *WDAClient) InputText(wdaURL string, sessionID string, text string) error {
	chars := make([]string, 0, len([]rune(text)))
	for _, r := range text {
		chars = append(chars, string(r))
	}

	body := map[string]interface{}{"value": chars}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("input text: marshal body: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/wda/keys", wdaURL, sessionID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("input text: %w", err)
	}
	defer resp.Body.Close()

	return c.checkResponse(resp, "input text")
}

// PressButton presses a hardware button (e.g., "home", "volumeUp") via WDA.
func (c *WDAClient) PressButton(wdaURL string, sessionID string, button string) error {
	body := map[string]interface{}{"name": button}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("press button: marshal body: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/wda/pressButton", wdaURL, sessionID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("press button: %w", err)
	}
	defer resp.Body.Close()

	return c.checkResponse(resp, "press button")
}

// LaunchApp launches an app by bundle ID via WDA's custom endpoint.
// This works without Developer Image / instruments.
func (c *WDAClient) LaunchApp(wdaURL string, sessionID string, bundleID string) error {
	body := map[string]interface{}{"bundleId": bundleID}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("launch app: marshal body: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/wda/apps/launch", wdaURL, sessionID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("launch app: %w", err)
	}
	defer resp.Body.Close()

	return c.checkResponse(resp, "launch app")
}

// KillApp terminates an app by bundle ID via WDA's custom endpoint.
func (c *WDAClient) KillApp(wdaURL string, sessionID string, bundleID string) error {
	body := map[string]interface{}{"bundleId": bundleID}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("kill app: marshal body: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/wda/apps/terminate", wdaURL, sessionID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("kill app: %w", err)
	}
	defer resp.Body.Close()

	return c.checkResponse(resp, "kill app")
}

// Screenshot takes a screenshot and returns the raw PNG bytes.
// It calls GET {wdaURL}/screenshot which returns base64-encoded image data.
func (c *WDAClient) Screenshot(wdaURL string, sessionID string) ([]byte, error) {
	url := wdaURL + "/screenshot"
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("screenshot: decode response: %w", err)
	}

	raw, err := base64.StdEncoding.DecodeString(result.Value)
	if err != nil {
		return nil, fmt.Errorf("screenshot: base64 decode: %w", err)
	}
	return raw, nil
}

// GetElementTree fetches the XML page source and parses it into WDAElements.
func (c *WDAClient) GetElementTree(wdaURL string, sessionID string) ([]driver.WDAElement, error) {
	xmlData, err := c.fetchSource(wdaURL, sessionID)
	if err != nil {
		return nil, err
	}
	elements, err := ParseSource(xmlData)
	if err != nil {
		return nil, fmt.Errorf("get element tree: %w", err)
	}
	return elements, nil
}

// GetInteractiveElements returns only elements matching the given type list.
// If types is empty, all elements are returned.
func (c *WDAClient) GetInteractiveElements(wdaURL string, sessionID string, types []string) ([]driver.WDAElement, error) {
	all, err := c.GetElementTree(wdaURL, sessionID)
	if err != nil {
		return nil, err
	}
	return FilterInteractive(all, types), nil
}

// FindElement locates a single element using the given strategy and value.
// Common strategies: "name", "accessibility id", "class name", "xpath".
func (c *WDAClient) FindElement(wdaURL string, sessionID string, using string, value string) (*driver.WDAElement, error) {
	body := map[string]string{"using": using, "value": value}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("find element: marshal body: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/element", wdaURL, sessionID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("find element: %w", err)
	}
	defer resp.Body.Close()

	// WDA returns {"value":{"ELEMENT":"<id>"}} — we resolve via page source.
	var result struct {
		Value map[string]string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("find element: decode response: %w", err)
	}

	// Check that an element was found (value map should have an entry).
	if len(result.Value) == 0 {
		return nil, fmt.Errorf("find element: element not found")
	}

	// Resolve element details from the page source by matching label/name.
	elements, err := c.GetElementTree(wdaURL, sessionID)
	if err != nil {
		return nil, fmt.Errorf("find element: get tree: %w", err)
	}
	for _, el := range elements {
		if el.Label == value {
			elCopy := el
			return &elCopy, nil
		}
	}

	// Return a minimal element when we can't resolve by label.
	el := &driver.WDAElement{Label: value}
	return el, nil
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

// checkResponse reads the WDA HTTP response and returns an error if the
// status code is not 2xx, including the WDA error message when available.
func (c *WDAClient) checkResponse(resp *http.Response, context string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)

	// Try to extract WDA error message from JSON response.
	var wdaResp struct {
		Value struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		} `json:"value"`
	}
	if json.Unmarshal(body, &wdaResp) == nil && wdaResp.Value.Message != "" {
		return fmt.Errorf("%s: WDA %d: %s: %s", context, resp.StatusCode, wdaResp.Value.Error, wdaResp.Value.Message)
	}
	return fmt.Errorf("%s: WDA returned HTTP %d: %s", context, resp.StatusCode, string(body))
}

// fetchSource calls GET /session/{id}/source and returns raw body bytes.
func (c *WDAClient) fetchSource(wdaURL, sessionID string) ([]byte, error) {
	url := fmt.Sprintf("%s/session/%s/source", wdaURL, sessionID)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch source: %w", err)
	}
	defer resp.Body.Close()

	// The source may be wrapped in a JSON envelope {"value":"<xml>"} or
	// may be raw XML. Handle both.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch source: read body: %w", err)
	}

	// Detect JSON envelope.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var envelope struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err == nil && envelope.Value != "" {
			return []byte(envelope.Value), nil
		}
	}

	return body, nil
}

// pointerAction is a single W3C pointer action step.
type pointerAction struct {
	Type     string `json:"type"`
	Duration int    `json:"duration,omitempty"`
	X        int    `json:"x,omitempty"`
	Y        int    `json:"y,omitempty"`
	Button   int    `json:"button,omitempty"`
}

// buildPointerActions constructs a W3C actions payload for a touch pointer.
func buildPointerActions(id string, steps []pointerAction) map[string]interface{} {
	// Use raw maps so we can include "button":0 on pointerDown/Up even when 0.
	rawSteps := make([]map[string]interface{}, len(steps))
	for i, s := range steps {
		m := map[string]interface{}{"type": s.Type}
		switch s.Type {
		case "pointerMove":
			m["duration"] = s.Duration
			m["x"] = s.X
			m["y"] = s.Y
		case "pointerDown", "pointerUp":
			m["button"] = s.Button
		case "pause":
			m["duration"] = s.Duration
		}
		rawSteps[i] = m
	}

	return map[string]interface{}{
		"actions": []map[string]interface{}{
			{
				"type": "pointer",
				"id":   id,
				"parameters": map[string]string{
					"pointerType": "touch",
				},
				"actions": rawSteps,
			},
		},
	}
}

// postActions sends a W3C actions payload to POST /session/{id}/actions.
func (c *WDAClient) postActions(wdaURL, sessionID string, actions map[string]interface{}) error {
	data, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("post actions: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/actions", wdaURL, sessionID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post actions: %w", err)
	}
	defer resp.Body.Close()

	return c.checkResponse(resp, "post actions")
}


