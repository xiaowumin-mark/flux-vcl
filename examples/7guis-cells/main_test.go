package main

import "testing"

func TestFormulaDependencyPropagationAndCycle(t *testing.T) {
	m := newBlankSheet(1, 3)
	a1 := cellCoord{row: 0, column: 0}
	b1 := cellCoord{row: 0, column: 1}
	c1 := cellCoord{row: 0, column: 2}

	m.setCell(a1, "1")
	m.setCell(b1, "=A1+2")
	m.setCell(c1, "=B1+3")
	if got := m.Display[0][2]; got != "6" {
		t.Fatalf("initial C1 = %q, want 6", got)
	}

	m.setCell(a1, "4")
	if got := m.Display[0][1]; got != "6" {
		t.Fatalf("propagated B1 = %q, want 6", got)
	}
	if got := m.Display[0][2]; got != "9" {
		t.Fatalf("propagated C1 = %q, want 9", got)
	}

	m.setCell(a1, "=C1+1")
	for column, got := range m.Display[0] {
		if got != cycleError {
			t.Fatalf("cycle column %d = %q, want %s", column, got, cycleError)
		}
	}
}

func TestFormulaErrorsRemainVisibleAndPropagate(t *testing.T) {
	m := newBlankSheet(1, 2)
	a1 := cellCoord{row: 0, column: 0}
	b1 := cellCoord{row: 0, column: 1}
	m.setCell(b1, "=A1+1")

	m.setCell(a1, "text")
	if got := m.Display[0][1]; got != valueError {
		t.Fatalf("text dependency = %q, want %s", got, valueError)
	}

	m.setCell(a1, "=Z99")
	if got := m.Display[0][0]; got != referenceErr {
		t.Fatalf("invalid reference = %q, want %s", got, referenceErr)
	}
	if got := m.Display[0][1]; got != referenceErr {
		t.Fatalf("dependent invalid reference = %q, want %s", got, referenceErr)
	}

	m.setCell(a1, "=1++2")
	if got := m.Display[0][0]; got != formulaError {
		t.Fatalf("parse error = %q, want %s", got, formulaError)
	}
}

func TestParseFormulaSupportsA1ReferencesAndAddition(t *testing.T) {
	terms, refs, errCode := parseFormula("= A1 + 2 + B2", 2, 2)
	if errCode != "" {
		t.Fatalf("parse error: %s", errCode)
	}
	if len(terms) != 3 || len(refs) != 2 {
		t.Fatalf("terms=%d refs=%d, want 3/2", len(terms), len(refs))
	}
}
