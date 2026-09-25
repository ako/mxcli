// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"
)

// A ColorPicker design property takes one of its swatches (stored as an Option)
// OR a custom colour (stored as Forms$CustomDesignPropertyValue) — the builder
// decides which in resolveDesignPropertyValueType. mxbuild accepts ANY string as
// a custom colour: 'banana', '#zzzzzz' and ' ' all built with 0 errors on Mendix
// 11.13.0. What happens to it is decided by the theme, and was read off the
// compiled page module (the PAD's web/dist/pages/<page>.js):
//
//   - the ColorPicker names a CSS `property` → the value is emitted verbatim as an
//     inline style (`style:{borderColor:"banana"}`), so it works exactly when it
//     is a CSS colour and the browser drops it otherwise;
//   - it names none → nothing is emitted at all, whatever the value.
//
// So "is this value allowed" has three answers on a ColorPicker, not two, and the
// check has to give the one that matches what reaches the page. Every other type
// takes only its options: the builder writes an off-list value as an Option,
// which mxbuild refuses (CE6085), and a Custom on a ToggleButtonGroup is refused
// too (CE6084, measured by forcing one).

// colorPickerValueProblem explains why value is not a working choice for the
// ColorPicker tp, or returns ok when it is (a swatch, or a CSS colour where the
// theme gives custom colours a CSS property).
func colorPickerValueProblem(tp *ThemeProperty, value string) (message, suggestion string, ok bool) {
	if themeOptionAllowed(tp.Options, value) {
		return "", "", true
	}
	swatches := themeOptionNames(tp.Options)

	// A swatch misspelt, or under a name the theme has since changed, is not a
	// colour anyone chose: written as a custom colour it renders as nothing.
	if cur := currentOptionName(tp.Options, value); cur != "" {
		return fmt.Sprintf("is a former name of the swatch %q — mxcli writes it as a custom colour, not as that swatch", cur),
			fmt.Sprintf("Write '%s'.", cur), false
	}
	for _, o := range tp.Options {
		if strings.EqualFold(o.Name, value) {
			return fmt.Sprintf("is not a swatch (values are case-sensitive) — did you mean %q? As written it becomes a custom colour", o.Name),
				fmt.Sprintf("Write '%s'.", o.Name), false
		}
	}

	if tp.Property == "" {
		return "is a custom colour that is not rendered: the theme gives this ColorPicker no CSS `property` to write it to, " +
				"so mxbuild accepts it and the page is built without it",
			fmt.Sprintf("Use one of its swatches: %s", swatches), false
	}
	if !isCSSColour(value) {
		return fmt.Sprintf("is not a CSS colour — it is written as a custom colour into %s, where the browser discards it", tp.Property),
			fmt.Sprintf("Use a swatch (%s) or a CSS colour such as '#ff0000' or 'rgb(255 0 0)'.", swatches), false
	}
	return "", "", true
}

var (
	cssHexColourRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	// A colour function, including var(): the arguments are not checked, only
	// that the call is one the browser treats as a colour and is closed.
	cssColourFuncRe = regexp.MustCompile(`(?i)^(rgba?|hsla?|hwb|lab|lch|oklab|oklch|color|color-mix|light-dark|var)\(.*\)$`)
)

// isCSSColour reports whether value is a CSS colour a browser would apply: a hex
// colour, a colour function (or var(), whose value cannot be known here), or a
// named colour.
func isCSSColour(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" || v != value {
		return false
	}
	if cssHexColourRe.MatchString(v) || cssColourFuncRe.MatchString(v) {
		return strings.Count(v, "(") == strings.Count(v, ")")
	}
	return cssNamedColours[strings.ToLower(v)]
}

// cssNamedColours is CSS Color Module Level 4's named colours plus the two
// keywords that stand for a colour.
var cssNamedColours = func() map[string]bool {
	names := strings.Fields(`transparent currentcolor
aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue
blueviolet brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk
crimson cyan darkblue darkcyan darkgoldenrod darkgray darkgreen darkgrey darkkhaki
darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen
darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue
dimgray dimgrey dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite
gold goldenrod gray green greenyellow grey honeydew hotpink indianred indigo ivory khaki
lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan
lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen
lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen
magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen
mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue mintcream
mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid
palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum
powderblue purple rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown
seagreen seashell sienna silver skyblue slateblue slategray slategrey snow springgreen
steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow
yellowgreen`)
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}()
