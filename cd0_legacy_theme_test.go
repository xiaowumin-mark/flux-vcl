package flux_test

import (
	"fmt"
	"reflect"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
)

// cd0LegacySnapshot is the executable CD0 reference model for the public
// FromLegacyTheme adapter planned in CD3.6. Keeping this type test-local avoids
// publishing a half-implemented DesignTheme before its resolver exists.
type cd0LegacySnapshot struct {
	primary      flux.ColorValue
	background   flux.ColorValue
	surface      flux.ColorValue
	foreground   flux.ColorValue
	accent       flux.ColorValue
	darkTitleBar bool
	fontSize     int
	radius       int
}

func cd0ReferenceFromLegacyTheme(legacy flux.Theme) cd0LegacySnapshot {
	fontSize := legacy.FontSize
	if fontSize < 0 {
		fontSize = 0
	}
	radius := legacy.Radius
	if radius < 0 {
		radius = 0
	}
	return cd0LegacySnapshot{
		primary:      legacy.Primary,
		background:   legacy.Background,
		surface:      legacy.Surface,
		foreground:   legacy.Text,
		accent:       legacy.Accent,
		darkTitleBar: legacy.DarkTitleBar,
		fontSize:     fontSize,
		radius:       radius,
	}
}

// legacyThemeWidget is the compile-checked pre-CD3 migration shape. Replacing
// these explicit opts with ThemeScope(FromLegacyTheme(theme), child) must keep
// the same input snapshot and NativeRendering compatibility behavior.
func legacyThemeWidget(theme flux.Theme) flux.Widget {
	return flux.Window(
		flux.Color(theme.Background),
		flux.DarkTitleBar(theme.DarkTitleBar),
		flux.Column(
			flux.Text("Theme migration", flux.FontColor(theme.Text)),
			flux.Button("Save", flux.Color(theme.Primary), flux.FontColor(theme.Text)),
		),
	)
}

func Example_legacyThemeMigration() {
	legacy := flux.LightTheme // copy the mutable package variable at the boundary
	mapped := cd0ReferenceFromLegacyTheme(legacy)
	_ = legacyThemeWidget(legacy)

	fmt.Printf("primary=%08X font=%d radius=%d dark=%t\n",
		uint32(mapped.primary), mapped.fontSize, mapped.radius, mapped.darkTitleBar)
	// Output:
	// primary=FF1E90FF font=14 radius=4 dark=false
}

func TestCD0LegacyThemeMappingContract(t *testing.T) {
	legacy := flux.Theme{
		Primary:      flux.ColorValue(0x80112233), // adapter preserves alpha; Draw validation rejects it later
		Background:   flux.ColorValue(0xFF445566),
		Surface:      0,
		Text:         flux.ColorValue(0xFF778899),
		Accent:       flux.ColorValue(0xFFAABBCC),
		DarkTitleBar: true,
		FontSize:     17,
		Radius:       6,
	}
	want := cd0LegacySnapshot{
		primary:      legacy.Primary,
		background:   legacy.Background,
		surface:      legacy.Surface,
		foreground:   legacy.Text,
		accent:       legacy.Accent,
		darkTitleBar: true,
		fontSize:     17,
		radius:       6,
	}
	if got := cd0ReferenceFromLegacyTheme(legacy); !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy mapping = %+v, want %+v", got, want)
	}
}

func TestCD0LegacyThemeMappingNormalizesUnsupportedMetrics(t *testing.T) {
	mapped := cd0ReferenceFromLegacyTheme(flux.Theme{FontSize: -1, Radius: -8})
	if mapped.fontSize != 0 || mapped.radius != 0 {
		t.Fatalf("normalized metrics = font %d, radius %d; want 0, 0", mapped.fontSize, mapped.radius)
	}
}

func TestCD0LegacyThemeMappingSnapshotsMutablePackageVariables(t *testing.T) {
	legacy := flux.LightTheme
	mapped := cd0ReferenceFromLegacyTheme(legacy)
	original := mapped.primary

	legacy.Primary = flux.RGB(1, 2, 3)
	if mapped.primary != original {
		t.Fatalf("mapped theme followed caller mutation: got %08X, want %08X", uint32(mapped.primary), uint32(original))
	}

	// Mapping DarkTheme must not share state with the mutable package variable.
	dark := cd0ReferenceFromLegacyTheme(flux.DarkTheme)
	copyOfDark := dark
	dark.primary = flux.RGB(9, 8, 7)
	if !reflect.DeepEqual(copyOfDark, cd0ReferenceFromLegacyTheme(flux.DarkTheme)) {
		t.Fatal("reference mapping mutated flux.DarkTheme or retained shared state")
	}
}
