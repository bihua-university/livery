package components

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/theme"
)

func TestSegmentedValueSelectionAndDisabled(t *testing.T) {
	type mode string
	const (
		modeRule   mode = "rule"
		modeGlobal mode = "global"
		modeDirect mode = "direct"
	)

	current := mode("unknown")
	disabled := false
	var changes []mode

	opts := []SegmentOption[mode]{
		{Value: modeRule, Label: "Rule"},
		{Value: modeGlobal, Label: "Global"},
		{Value: modeDirect, Label: "Direct"},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		p := theme.Light()
		s := theme.Scale(14)
		c.SetTheme(theme.BuildUITheme(p, 14))
		ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
			SegmentedValue(c.Key("mode-seg"), p, s, current, opts, func(next mode) {
				current = next
				changes = append(changes, next)
			}).Disabled(disabled).FillEqually().Grow(1)
			ui.Text(c, "selected:"+string(current))
		})
	}, 420, 140)

	// Clicking "Global" should invoke onChange with modeGlobal and update the rendered text.
	if err := tester.Click("Global"); err != nil {
		t.Fatalf("Click(Global): %v", err)
	}
	if current != modeGlobal || len(changes) != 1 || changes[0] != modeGlobal {
		t.Fatalf("expected current=global and 1 change, got current=%q changes=%v", current, changes)
	}
	if !tester.HasText("selected:global") {
		t.Fatalf("expected updated text selected:global, texts=%v", tester.Texts())
	}

	// Additional frames must not re-fire onChange.
	tester.Frame()
	if len(changes) != 1 {
		t.Fatalf("extra frame re-fired onChange: %v", changes)
	}

	// Chaining .Disabled(true) after SegmentedValue must block clicks and onChange.
	disabled = true
	tester.Frame()
	if err := tester.Click("Direct"); err != nil {
		t.Fatalf("Click(Direct) while disabled: %v", err)
	}
	if current != modeGlobal || len(changes) != 1 {
		t.Fatalf("expected disabled SegmentedValue to ignore click, got current=%q changes=%v", current, changes)
	}

	// Re-enabling allows subsequent selection changes.
	disabled = false
	tester.Frame()
	if err := tester.Click("Direct"); err != nil {
		t.Fatalf("Click(Direct) after re-enable: %v", err)
	}
	if current != modeDirect || len(changes) != 2 || changes[1] != modeDirect {
		t.Fatalf("expected current=direct after re-enable, got current=%q changes=%v", current, changes)
	}
}

func TestSegmentedValueEmptyOptions(t *testing.T) {
	called := false
	tester := ui.NewTester(func(c *ui.Context) {
		p := theme.Dark()
		s := theme.Scale(14)
		SegmentedValue(c.Key("empty-seg"), p, s, "", []SegmentOption[string]{}, func(string) {
			called = true
		})
	}, 200, 100)
	tester.Frame()
	if called {
		t.Fatal("expected empty SegmentedValue not to fire onChange")
	}
}
