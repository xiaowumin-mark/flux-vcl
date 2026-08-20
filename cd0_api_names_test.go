package flux_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type cd0IdentifierKind string

const (
	cd0Type  cd0IdentifierKind = "type"
	cd0Func  cd0IdentifierKind = "func"
	cd0Var   cd0IdentifierKind = "var"
	cd0Const cd0IdentifierKind = "const"
)

type cd0Identifier struct {
	name string
	kind cd0IdentifierKind
}

// These names are reserved by docs/api-vnext.md but intentionally do not
// become public until their CD1-CD4 implementation stage. Once a symbol is
// implemented, move it to cd0ReusedIdentifiers so the audit keeps checking its
// declaration kind without treating the intended implementation as a clash.
var cd0ReservedIdentifiers = []cd0Identifier{
	// CD2.1 is implemented; these names are checked below as intentional reuse.
	{"DesignTheme", cd0Type},
	{"ColorScheme", cd0Type},
	{"Typography", cd0Type},
	{"MetricTokens", cd0Type},
	{"ComponentThemes", cd0Type},
	{"SurfaceTheme", cd0Type},
	{"TextTheme", cd0Type},
	{"ButtonTheme", cd0Type},
	{"InputTheme", cd0Type},
	{"CheckBoxTheme", cd0Type},
	{"RadioTheme", cd0Type},
	{"ComboBoxTheme", cd0Type},
	{"ProgressTheme", cd0Type},
	{"SliderTheme", cd0Type},
	{"GridTheme", cd0Type},
	{"TabTheme", cd0Type},
	{"FromLegacyTheme", cd0Func},
	{"ThemeScope", cd0Func},
	{"DrawSurface", cd0Func},
	{"Surface", cd0Func},
}

var cd0ReservedConstants = []string{}

// Existing API names are deliberately reused by vNext. Their kind is part of
// the decision: in particular Color must remain the Opt function, while the
// ARGB value type remains ColorValue.
var cd0ReusedIdentifiers = []cd0Identifier{
	{"Insets", cd0Type},
	{"BorderSpec", cd0Type},
	{"ControlStyle", cd0Type},
	{"FocusStyle", cd0Type},
	{"StyleFieldMask", cd0Type},
	{"ControlStylePatch", cd0Type},
	{"DrawList", cd0Type},
	{"DrawOp", cd0Type},
	{"FillStyle", cd0Type},
	{"StrokeKind", cd0Type},
	{"StrokeStyle", cd0Type},
	{"FontWeight", cd0Type},
	{"FontSpec", cd0Type},
	{"TextAlignment", cd0Type},
	{"TextWrap", cd0Type},
	{"TextOverflow", cd0Type},
	{"TextPaint", cd0Type},
	{"DrawValidationError", cd0Type},
	{"ErrInvalidDrawList", cd0Var},
	{"NewDrawList", cd0Func},
	{"MustDrawList", cd0Func},
	{"Clear", cd0Func},
	{"FillRect", cd0Func},
	{"StrokeRect", cd0Func},
	{"FillRoundRect", cd0Func},
	{"StrokeRoundRect", cd0Func},
	{"DrawLine", cd0Func},
	{"FillEllipse", cd0Func},
	{"StrokeEllipse", cd0Func},
	{"DrawText", cd0Func},
	{"PushClip", cd0Func},
	{"PopClip", cd0Func},
	{"StrokeSolid", cd0Const},
	{"FontWeightNormal", cd0Const},
	{"FontWeightMedium", cd0Const},
	{"FontWeightSemibold", cd0Const},
	{"FontWeightBold", cd0Const},
	{"TextAlignStart", cd0Const},
	{"TextAlignCenter", cd0Const},
	{"TextAlignEnd", cd0Const},
	{"TextNoWrap", cd0Const},
	{"TextWrapWord", cd0Const},
	{"TextOverflowClip", cd0Const},
	{"TextOverflowEllipsis", cd0Const},
	{"ColorValue", cd0Type},
	{"Point", cd0Type},
	{"Rect", cd0Type},
	{"Size", cd0Type},
	{"Widget", cd0Type},
	{"Opt", cd0Type},
	{"Theme", cd0Type},
	{"Color", cd0Func},
	{"FontColor", cd0Func},
	{"PaintBox", cd0Func},
	{"PaintCommand", cd0Type},
	{"LightTheme", cd0Var},
	{"DarkTheme", cd0Var},
}

func TestCD0VNextNamesDoNotConflictWithRootPackage(t *testing.T) {
	declared := cd0RootPackageIdentifiers(t)
	seen := make(map[string]string)

	for _, identifier := range cd0ReservedIdentifiers {
		cd0CheckReservedIdentifier(t, declared, seen, identifier, "api-vnext")
	}
	for _, name := range cd0ReservedConstants {
		cd0CheckReservedIdentifier(t, declared, seen, cd0Identifier{name, cd0Const}, "api-vnext constant")
	}
}

func TestCD0VNextIntentionalReuseKeepsDeclarationKind(t *testing.T) {
	declared := cd0RootPackageIdentifiers(t)
	for _, identifier := range cd0ReusedIdentifiers {
		got, ok := declared[identifier.name]
		if !ok {
			t.Errorf("CD0 reused identifier %s is no longer declared", identifier.name)
			continue
		}
		if got != identifier.kind {
			t.Errorf("CD0 reused identifier %s is %s, want %s", identifier.name, got, identifier.kind)
		}
	}
}

func cd0CheckReservedIdentifier(
	t *testing.T,
	declared map[string]cd0IdentifierKind,
	seen map[string]string,
	identifier cd0Identifier,
	owner string,
) {
	t.Helper()
	if !token.IsIdentifier(identifier.name) || !ast.IsExported(identifier.name) {
		t.Errorf("CD0 %s name %q is not a valid exported Go identifier", owner, identifier.name)
	}
	if previous, ok := seen[identifier.name]; ok {
		t.Errorf("CD0 name %s is reserved twice (%s and %s)", identifier.name, previous, owner)
	} else {
		seen[identifier.name] = owner
	}
	if got, ok := declared[identifier.name]; ok {
		t.Errorf("CD0 name %s (%s) conflicts with existing root-package %s; if this is the intended implementation, move it to cd0ReusedIdentifiers", identifier.name, identifier.kind, got)
	}
}

func cd0RootPackageIdentifiers(t *testing.T) map[string]cd0IdentifierKind {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate CD0 API audit source")
	}
	directory := filepath.Dir(source)
	files, err := parser.ParseDir(token.NewFileSet(), directory, func(info os.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse root package: %v", err)
	}
	pkg, ok := files["flux"]
	if !ok {
		t.Fatalf("root package flux not found in %s", directory)
	}

	declared := make(map[string]cd0IdentifierKind)
	for _, file := range pkg.Files {
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Recv == nil {
					declared[value.Name.Name] = cd0Func
				}
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					switch spec := specification.(type) {
					case *ast.TypeSpec:
						declared[spec.Name.Name] = cd0Type
					case *ast.ValueSpec:
						kind := cd0Var
						if value.Tok == token.CONST {
							kind = cd0Const
						}
						for _, name := range spec.Names {
							declared[name.Name] = kind
						}
					}
				}
			}
		}
	}
	return declared
}
