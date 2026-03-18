package wda

import (
	"encoding/xml"
	"fmt"
	"strings"

	"ios-pilot/internal/driver"
)

// xmlNode is a flexible representation of a WDA XML element.
// It supports arbitrary attributes and nested children.
type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []xmlNode  `xml:",any"`
}

// attr returns the value of the named XML attribute, or "" if not found.
func (n *xmlNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// ParseSource parses WDA XML source into a flat list of driver.WDAElement.
// It walks all nodes recursively, skipping container nodes that carry no
// useful UI information (e.g., AppiumAUT, XCUIElementTypeApplication).
func ParseSource(xmlData []byte) ([]driver.WDAElement, error) {
	var root xmlNode
	if err := xml.Unmarshal(xmlData, &root); err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	var elements []driver.WDAElement
	collectElements(&root, &elements)
	return elements, nil
}

// collectElements walks the XML tree depth-first and appends elements.
func collectElements(node *xmlNode, out *[]driver.WDAElement) {
	name := node.XMLName.Local
	if name != "" && name != "AppiumAUT" {
		el, ok := nodeToElement(node)
		if ok {
			*out = append(*out, el)
		}
	}
	for i := range node.Children {
		collectElements(&node.Children[i], out)
	}
}

// nodeToElement converts an xmlNode to a driver.WDAElement.
// Returns (element, false) if the node has no meaningful position info.
func nodeToElement(node *xmlNode) (driver.WDAElement, bool) {
	typeName := node.XMLName.Local

	x := atoi(node.attr("x"))
	y := atoi(node.attr("y"))
	w := atoi(node.attr("width"))
	h := atoi(node.attr("height"))

	label := node.attr("name")
	if label == "" {
		label = node.attr("label")
	}
	if label == "" {
		label = node.attr("value")
	}

	el := driver.WDAElement{
		Type:   mapTypeName(typeName),
		Label:  label,
		Frame:  [4]int{x, y, w, h},
		Center: [2]int{x + w/2, y + h/2},
	}
	return el, true
}

// mapTypeName converts an XCUIElementType* name to a short, normalised type string.
func mapTypeName(xcuiName string) string {
	switch xcuiName {
	case "XCUIElementTypeButton":
		return "button"
	case "XCUIElementTypeTextField", "XCUIElementTypeSecureTextField":
		return "textfield"
	case "XCUIElementTypeSwitch":
		return "switch"
	case "XCUIElementTypeLink":
		return "link"
	case "XCUIElementTypeCell":
		return "cell"
	case "XCUIElementTypeStaticText":
		return "text"
	default:
		// Strip the "XCUIElementType" prefix and return lowercase.
		const prefix = "XCUIElementType"
		if strings.HasPrefix(xcuiName, prefix) {
			return strings.ToLower(xcuiName[len(prefix):])
		}
		return strings.ToLower(xcuiName)
	}
}

// FilterInteractive returns the subset of elements whose type is in the
// provided types list. If types is empty, all elements are returned unchanged.
func FilterInteractive(elements []driver.WDAElement, types []string) []driver.WDAElement {
	if len(types) == 0 {
		return elements
	}

	allowed := make(map[string]struct{}, len(types))
	for _, t := range types {
		allowed[t] = struct{}{}
	}

	out := make([]driver.WDAElement, 0)
	for _, el := range elements {
		if _, ok := allowed[el.Type]; ok {
			out = append(out, el)
		}
	}
	return out
}

// atoi converts a string to int, returning 0 on error.
func atoi(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	neg := false
	i := 0
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		return -n
	}
	return n
}
