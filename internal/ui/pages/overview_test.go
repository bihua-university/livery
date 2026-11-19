package pages

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/profile"
	"github.com/bihua-university/livery/internal/theme"
)

func TestSampleMonotoneSeries(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 200, H: 100}
	xs := []float32{0, 0.25, 0.5, 0.75, 1}
	zeros := []float32{0, 0, 0, 0, 0}

	var pts [][2]float32
	var slopes []float32
	pts = sampleMonotoneSeries(pts, &slopes, r, xs, zeros, false)
	if len(pts) != 2 {
		t.Fatalf("expected 2 points for flat zero series, got %d", len(pts))
	}
	if pts[0][1] != r.Y+r.H || pts[1][1] != r.Y+r.H {
		t.Fatalf("expected baseline points at bottom=%v, got %+v", r.Y+r.H, pts)
	}

	vals := []float32{0, 0.6, 0.2, 1.0, 0.1}
	pts = sampleMonotoneSeries(pts, &slopes, r, xs, vals, true)
	if len(pts) <= len(xs) {
		t.Fatalf("expected subdivided monotone curve points > %d, got %d", len(xs), len(pts))
	}
	for i, pt := range pts {
		if pt[1] < r.Y || pt[1] > r.Y+r.H {
			t.Fatalf("point %d out of vertical bounds [%v, %v]: %+v", i, r.Y, r.Y+r.H, pt)
		}
		if i > 0 && pt[0] < pts[i-1][0] {
			t.Fatalf("x coordinates must be non-decreasing at %d: %v < %v", i, pt[0], pts[i-1][0])
		}
	}

	capBefore := cap(pts)
	pts = sampleMonotoneSeries(pts, &slopes, r, xs, vals, true)
	if cap(pts) != capBefore {
		t.Fatalf("expected point slice capacity reuse (%d), got %d", capBefore, cap(pts))
	}
}

func TestRulesAndLogsLoadingStateRender(t *testing.T) {
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	cm := core.NewManager(dir, dir, nil, nil)
	a := app.New(pm, cm)
	t.Cleanup(func() {
		a.Shutdown()
	})
	a.SetMainRunner(func(fn func()) {
		// Hold async completions so we can assert the loading spinner state deterministically.
	})

	a.Page = app.PageRules
	rulesTester := ui.NewTester(func(c *ui.Context) {
		Rules(c, a, theme.Light(), theme.Scale(14))
	}, 960, 700)
	if !rulesTester.HasText("正在加载分流规则...") {
		t.Fatal("expected Rules page to show loading state while initial async load is in flight")
	}

	a.Page = app.PageLogs
	logsTester := ui.NewTester(func(c *ui.Context) {
		Logs(c, a, theme.Light(), theme.Scale(14))
	}, 960, 700)
	if !logsTester.HasText("正在加载运行日志...") {
		t.Fatal("expected Logs page to show loading state while initial async load is in flight")
	}
}
