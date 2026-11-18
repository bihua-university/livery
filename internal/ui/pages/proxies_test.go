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

func TestProxyGroupHeaderFullHoverAndNoInitialBox(t *testing.T) {
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	cm := core.NewManager(dir, dir, nil, nil)
	a := app.New(pm, cm)
	t.Cleanup(a.Shutdown)
	a.CoreInfo.Status = model.CoreRunning
	a.ExpandedGroups = map[string]bool{
		"Primary":   true,
		"Secondary": false,
	}
	a.Groups = []model.ProxyGroup{
		{Name: "Primary", Type: "Selector", Now: "NodeA", All: []string{"NodeA"}},
		{Name: "Secondary", Type: "Selector", Now: "NodeB", All: []string{"NodeB"}},
	}
	a.Nodes = map[string]model.ProxyNode{
		"NodeA": {Name: "NodeA", Type: "Direct", Delay: 10},
		"NodeB": {Name: "NodeB", Type: "Direct", Delay: 20},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		Proxies(c, a, theme.Light(), theme.Scale(14))
	}, 960, 700)

	// Generated single-letter initial boxes ("P", "S") should not be rendered.
	for _, txt := range tester.Texts() {
		if txt == "P" || txt == "S" {
			t.Fatalf("unexpected generated initial square badge %q in texts: %v", txt, tester.Texts())
		}
	}

	// Clicking the nested speed test button inside an expanded header must not collapse the group.
	if err := tester.Click("组测速 Primary"); err != nil {
		t.Fatalf("Click(组测速 Primary): %v", err)
	}
	if !a.IsGroupExpanded("Primary") {
		t.Fatal("expected Primary to remain expanded when clicking speed test button")
	}

	// Verify hovering over the right side ("Selector" label) highlights the entire header,
	// including both the left and right ends of the Secondary header row.
	secHeader, ok := tester.Find("Secondary")
	if !ok {
		t.Fatal("expected Secondary header element")
	}
	selBadge, ok := tester.Find("Selector")
	if !ok {
		t.Fatal("expected Selector text inside Secondary header")
	}
	sampleY := int(secHeader.Y + 4)
	leftX := int(secHeader.X + 4)
	rightX := int(secHeader.X + secHeader.W - 4)

	tester.Move(0, 0)
	baseImg := tester.Image()
	baseLeft := baseImg.RGBAAt(leftX, sampleY)
	baseRight := baseImg.RGBAAt(rightX, sampleY)

	tester.Move(selBadge.X+selBadge.W/2, selBadge.Y+selBadge.H/2)
	hoverImg := tester.Image()
	hoverLeft := hoverImg.RGBAAt(leftX, sampleY)
	hoverRight := hoverImg.RGBAAt(rightX, sampleY)

	if hoverLeft == baseLeft {
		t.Fatalf("expected left side of header to highlight when hovering right side (remained %v)", hoverLeft)
	}
	if hoverRight == baseRight {
		t.Fatalf("expected right side of header to highlight when hovering right side (remained %v)", hoverRight)
	}
}

