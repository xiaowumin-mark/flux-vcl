package render

import (
	"errors"
	"testing"
)

func TestValidatePaintCommandsRejectsPartialAlpha(t *testing.T) {
	tests := []struct {
		name    string
		command PaintCommand
	}{
		{
			name:    "clear",
			command: PaintCommand{Kind: PaintClear, Color: Color(0x80112233)},
		},
		{
			name:    "zero alpha with nonzero rgb",
			command: PaintCommand{Kind: PaintClear, Color: Color(0x00112233)},
		},
		{
			name: "fill",
			command: PaintCommand{
				Kind: PaintCircle, Radius: 8, FillColor: Color(0x80112233),
			},
		},
		{
			name: "stroke",
			command: PaintCommand{
				Kind: PaintCircle, Radius: 8, StrokeColor: Color(0x80112233), StrokeWidth: 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePaintCommands([]PaintCommand{test.command})
			var validation *PaintValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %T %v, want *PaintValidationError", err, err)
			}
			if validation.Kind != PaintValidationPartialAlpha || validation.Index != 0 {
				t.Fatalf("validation = %+v", validation)
			}
		})
	}
}

func TestValidatePaintCommandsAlphaBoundary(t *testing.T) {
	commands := []PaintCommand{
		{Kind: PaintClear, Color: RGB(0x11, 0x22, 0x33)},
		{Kind: PaintCircle, Radius: 8, FillColor: RGB(0x11, 0x22, 0x33)},
		{Kind: PaintCircle, Radius: 8, StrokeColor: RGB(0x11, 0x22, 0x33), StrokeWidth: 1},
	}
	for _, command := range commands {
		if err := ValidatePaintCommands([]PaintCommand{command}); err != nil {
			t.Fatalf("command %+v: %v", command, err)
		}
	}
}

func TestValidatePaintCommandsIgnoresUnusedColorFields(t *testing.T) {
	commands := []PaintCommand{
		{Kind: PaintClear, Color: RGB(1, 2, 3), FillColor: Color(0x80112233)},
		{Kind: PaintCircle, Radius: 8, Color: Color(0x80112233), FillColor: RGB(1, 2, 3)},
	}
	if err := ValidatePaintCommands(commands); err != nil {
		t.Fatalf("unused legacy PaintCommand fields changed validation: %v", err)
	}
}
