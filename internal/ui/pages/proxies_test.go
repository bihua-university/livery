package pages

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/profile"
	"github.com/bihua-university/livery/internal/theme"
)

func TestProxyGroupAccordionAnimation(t *testing.T) {
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
	a.CoreInfo.Status = model.CoreRunning
	a.ExpandedGroups = map[string]bool{
		"Primary":   true,
		"Secondary": false,
	}
	a.Groups = []model.ProxyGroup{
		{Name: "Primary", Type: "Selector", Now: "NodeA", All: []string{"NodeA", "NodeB"}},
		{Name: "Secondary", Type: "Selector", Now: "NodeC", All: []string{"NodeC", "NodeD", "NodeE", "NodeF"}},
	}
	a.Nodes = map[string]model.ProxyNode{
		"NodeA": {Name: "NodeA", Type: "Shadowsocks", Delay: 50},
		"NodeB": {Name: "NodeB", Type: "VMess", Delay: 80},
		"NodeC": {Name: "NodeC", Type: "Trojan", Delay: 40},
		"NodeD": {Name: "NodeD", Type: "VLESS", Delay: 60},
		"NodeE": {Name: "NodeE", Type: "Hysteria2", Delay: 90},
		"NodeF": {Name: "NodeF", Type: "Direct", Delay: 10},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		Proxies(c, a, theme.Light(), theme.Scale(14))
	}, 960, 700)

	// Initially Primary is expanded, Secondary is collapsed (NodeD in body should not be rendered).
	if tester.HasText("NodeD") {
		t.Fatal("expected Secondary group to start collapsed without NodeD rendered")
	}

	// Click Secondary toggle to expand it and start the animation.
	if err := tester.Click("Secondary"); err != nil {
		t.Fatalf("Click(Secondary): %v", err)
	}
	if !a.IsGroupExpanded("Secondary") {
		t.Fatal("expected Secondary to be marked expanded after click")
	}
	if !tester.HasText("NodeD") {
		t.Fatal("expected NodeD to enter the animated clip container during expansion")
	}

	// Advance partway through the 220ms animation and verify it is animating smoothly.
	tester.Advance(110 * time.Millisecond)
	midNode, ok := tester.Find("NodeD")
	if !ok {
		t.Fatal("expected NodeD to be measurable mid-animation")
	}
	if !tester.FramePending() {
		t.Fatal("expected pending animation frame mid-expansion")
	}

	// Finish the animation and verify Secondary is settled at full height.
	tester.Advance(1000 * time.Millisecond)
	if tester.FramePending() {
		t.Fatal("expected animation to settle with no pending frames after 1s")
	}
	endNode, ok := tester.Find("NodeD")
	if !ok || endNode.H <= 0 || endNode.H < midNode.H {
		t.Fatalf("unexpected NodeD bounds after animation settled: end=%+v mid=%+v", endNode, midNode)
	}

	// Collapse Secondary and verify body remains rendered mid-collapse and is removed once settled.
	if err := tester.Click("Secondary"); err != nil {
		t.Fatalf("Click(Secondary) to collapse: %v", err)
	}
	if a.IsGroupExpanded("Secondary") {
		t.Fatal("expected Secondary to be marked collapsed after second click")
	}
	tester.Advance(80 * time.Millisecond)
	if !tester.HasText("NodeD") {
		t.Fatal("expected NodeD to remain visible inside clip while collapse animation is in progress")
	}

	tester.Advance(300 * time.Millisecond)
	if tester.HasText("NodeD") {
		t.Fatal("expected NodeD to be removed once collapse animation finishes")
	}
}
