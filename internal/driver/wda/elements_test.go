package wda

import (
	"testing"

	"ios-pilot/internal/driver"
)

const sampleXML = `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="MyApp" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="Login" x="150" y="400" width="90" height="44"/>
    <XCUIElementTypeTextField type="XCUIElementTypeTextField" name="Username" x="50" y="300" width="290" height="40"/>
    <XCUIElementTypeSecureTextField type="XCUIElementTypeSecureTextField" name="Password" x="50" y="350" width="290" height="40"/>
    <XCUIElementTypeSwitch type="XCUIElementTypeSwitch" name="RememberMe" x="50" y="460" width="51" height="31"/>
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="Welcome" x="100" y="100" width="190" height="30"/>
    <XCUIElementTypeCell type="XCUIElementTypeCell" name="Item1" x="0" y="500" width="390" height="44"/>
    <XCUIElementTypeLink type="XCUIElementTypeLink" name="ForgotPassword" x="120" y="460" width="150" height="20"/>
    <XCUIElementTypeImage type="XCUIElementTypeImage" name="Logo" x="100" y="50" width="100" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`

// ---------------------------------------------------------------------------
// ParseSource
// ---------------------------------------------------------------------------

func TestParseSource_Basic(t *testing.T) {
	elements, err := ParseSource([]byte(sampleXML))
	if err != nil {
		t.Fatalf("ParseSource error: %v", err)
	}
	if len(elements) == 0 {
		t.Fatal("expected non-empty elements slice")
	}
}

func TestParseSource_TypeMapping(t *testing.T) {
	elements, err := ParseSource([]byte(sampleXML))
	if err != nil {
		t.Fatalf("ParseSource error: %v", err)
	}

	// Build a map by label for easy lookup.
	byLabel := make(map[string]driver.WDAElement)
	for _, el := range elements {
		byLabel[el.Label] = el
	}

	cases := []struct {
		label    string
		wantType string
	}{
		{"Login", "button"},
		{"Username", "textfield"},
		{"Password", "textfield"},
		{"RememberMe", "switch"},
		{"Welcome", "text"},
		{"Item1", "cell"},
		{"ForgotPassword", "link"},
		{"Logo", "image"}, // unknown → lowercased "image"
	}

	for _, tc := range cases {
		el, ok := byLabel[tc.label]
		if !ok {
			t.Errorf("element %q not found", tc.label)
			continue
		}
		if el.Type != tc.wantType {
			t.Errorf("element %q type: got %q, want %q", tc.label, el.Type, tc.wantType)
		}
	}
}

func TestParseSource_Frame(t *testing.T) {
	elements, err := ParseSource([]byte(sampleXML))
	if err != nil {
		t.Fatal(err)
	}

	byLabel := make(map[string]driver.WDAElement)
	for _, el := range elements {
		byLabel[el.Label] = el
	}

	loginBtn, ok := byLabel["Login"]
	if !ok {
		t.Fatal("Login button not found")
	}

	// x=150 y=400 width=90 height=44
	wantFrame := [4]int{150, 400, 90, 44}
	if loginBtn.Frame != wantFrame {
		t.Errorf("Login frame: got %v, want %v", loginBtn.Frame, wantFrame)
	}
}

func TestParseSource_Center(t *testing.T) {
	elements, err := ParseSource([]byte(sampleXML))
	if err != nil {
		t.Fatal(err)
	}

	byLabel := make(map[string]driver.WDAElement)
	for _, el := range elements {
		byLabel[el.Label] = el
	}

	loginBtn := byLabel["Login"]
	// x=150, y=400, w=90, h=44 → center = (150+45, 400+22) = (195, 422)
	wantCenter := [2]int{195, 422}
	if loginBtn.Center != wantCenter {
		t.Errorf("Login center: got %v, want %v", loginBtn.Center, wantCenter)
	}

	usernameField := byLabel["Username"]
	// x=50, y=300, w=290, h=40 → center = (50+145, 300+20) = (195, 320)
	wantCenter2 := [2]int{195, 320}
	if usernameField.Center != wantCenter2 {
		t.Errorf("Username center: got %v, want %v", usernameField.Center, wantCenter2)
	}
}

func TestParseSource_InvalidXML(t *testing.T) {
	_, err := ParseSource([]byte(`<broken>`))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

// ---------------------------------------------------------------------------
// FilterInteractive
// ---------------------------------------------------------------------------

func TestFilterInteractive_EmptyTypes(t *testing.T) {
	elements, _ := ParseSource([]byte(sampleXML))
	filtered := FilterInteractive(elements, nil)
	if len(filtered) != len(elements) {
		t.Errorf("empty types should return all %d elements, got %d", len(elements), len(filtered))
	}
}

func TestFilterInteractive_ButtonOnly(t *testing.T) {
	elements, _ := ParseSource([]byte(sampleXML))
	filtered := FilterInteractive(elements, []string{"button"})
	if len(filtered) == 0 {
		t.Fatal("expected at least one button")
	}
	for _, el := range filtered {
		if el.Type != "button" {
			t.Errorf("unexpected type %q in button-only filter", el.Type)
		}
	}
}

func TestFilterInteractive_MultipleTypes(t *testing.T) {
	elements, _ := ParseSource([]byte(sampleXML))
	types := []string{"button", "textfield"}
	filtered := FilterInteractive(elements, types)

	allowed := map[string]bool{"button": true, "textfield": true}
	for _, el := range filtered {
		if !allowed[el.Type] {
			t.Errorf("unexpected type %q in filtered results", el.Type)
		}
	}

	// Must include Login (button) + Username + Password (textfields).
	if len(filtered) < 3 {
		t.Errorf("expected >= 3 elements, got %d", len(filtered))
	}
}

func TestFilterInteractive_NoMatch(t *testing.T) {
	elements, _ := ParseSource([]byte(sampleXML))
	filtered := FilterInteractive(elements, []string{"nonexistent"})
	if len(filtered) != 0 {
		t.Errorf("expected 0 matches, got %d", len(filtered))
	}
}

// ---------------------------------------------------------------------------
// mapTypeName (via ParseSource)
// ---------------------------------------------------------------------------

func TestMapTypeName_UnknownType(t *testing.T) {
	xml := `<AppiumAUT>
    <XCUIElementTypeScrollView type="XCUIElementTypeScrollView" name="scrollview1" x="0" y="0" width="100" height="100"/>
  </AppiumAUT>`

	elements, err := ParseSource([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(elements) == 0 {
		t.Fatal("expected at least one element")
	}
	// XCUIElementTypeScrollView → "scrollview"
	if elements[0].Type != "scrollview" {
		t.Errorf("type: got %q, want %q", elements[0].Type, "scrollview")
	}
}
