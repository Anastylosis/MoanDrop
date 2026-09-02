package core

import "testing"

func TestGeneratedLabel(t *testing.T) {
	cases := []struct {
		generated bool
		source    string
		want      string
	}{
		{false, "", LabelHuman},
		{false, GeneratedSourceDeclared, LabelHuman}, // source without the flag is meaningless: never a badge
		{true, "", LabelGenerated},                   // a server predating generated_source: detection
		{true, GeneratedSourceProvenance, LabelGenerated},
		{true, GeneratedSourceDeclared, LabelDeclaredGenerated},
	}
	for _, c := range cases {
		if got := GeneratedLabel(c.generated, c.source); got != c.want {
			t.Errorf("GeneratedLabel(%v, %q) = %q, want %q", c.generated, c.source, got, c.want)
		}
	}
	if LabelGenerated == LabelDeclaredGenerated {
		t.Error("a declaration must never read as a detection")
	}
}

func TestCreditLine(t *testing.T) {
	if got := CreditLine(""); got != "" {
		t.Errorf("CreditLine(\"\") = %q, want empty", got)
	}
	if got := CreditLine("somebody"); got != "by somebody" {
		t.Errorf("CreditLine = %q, want the release page's own wording", got)
	}
}
