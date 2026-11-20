package pages

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

func qualityModal(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	if !a.ShowQualityModal {
		return
	}
	components.ModalShell(c.Key("egress-quality"), p, &a.ShowQualityModal, 500, func() {
		ui.Text(c, a.Tr("节点质量检测", "Egress quality")).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground)
		report := a.QualityReport
		switch {
		case a.QualityChecking():
			ui.Text(c, a.Tr("正在通过代理检测当前出口…", "Checking the current route through the proxy…")).TextColor(p.ForegroundSubtle)
		case report.CheckedAt.IsZero():
			ui.Text(c, a.Tr("请在核心运行时检测当前出口。", "Run the check while the core is active.")).TextColor(p.ForegroundSubtle)
		default:
			ui.Text(c, a.Tr("检测时间 ", "Checked at ")+report.CheckedAt.Format("15:04:05")).FontSize(s.XS).TextColor(p.ForegroundSubtle)
			if report.IP != "" {
				parts := []string{report.IP}
				for _, place := range []string{report.Quality.Location.Country, report.Quality.Location.City} {
					if place != "" {
						parts = append(parts, place)
					}
				}
				ui.Text(c, strings.Join(parts, " · ")).FontSize(s.LG).TextColor(p.Foreground)
			}
			risk := report.Quality.Risk
			if report.IPError != nil {
				ui.Text(c, a.Tr("出口信息查询失败", "Egress lookup failed")).Tooltip(report.IPError.Error()).TextColor(p.Warning)
			} else if risk.Score == nil {
				ui.Text(c, a.Tr("IPQuery 风险评分暂无数据", "IPQuery risk score unavailable")).TextColor(p.ForegroundSubtle)
			} else {
				parts := []string{fmt.Sprintf(a.Tr("IPQuery 风险评分 %d/100", "IPQuery risk score %d/100"), *risk.Score)}
				if risk.VPN {
					parts = append(parts, "VPN")
				}
				if risk.Proxy {
					parts = append(parts, a.Tr("代理", "Proxy"))
				}
				if risk.Tor {
					parts = append(parts, "Tor")
				}
				if risk.Datacenter {
					parts = append(parts, a.Tr("数据中心", "Datacenter"))
				}
				ui.Text(c, strings.Join(parts, " · ")).FontSize(s.SM).TextColor(p.Foreground)
			}
			ui.Divider(c)
			for _, service := range report.Services {
				ui.Row(c.Key(service.Name)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, service.Name).Grow(1).TextColor(p.Foreground)
					label, color := a.Tr("未收到响应", "No response"), p.ForegroundSubtle
					if service.Status != 0 {
						label = fmt.Sprintf("HTTP %d · %d ms", service.Status, service.Elapsed.Milliseconds())
						color = p.Success
						if service.Status >= 400 {
							color = p.Warning
						}
					}
					ui.Text(c, label).FontSize(s.SM).TextColor(color)
				})
			}
		}
		ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
			if components.ActionButton(c.Key("quality-retry"), p, s, nil, a.Tr("重新检测", "Check again")).Disabled(a.QualityChecking() || a.CoreInfo.Status != model.CoreRunning || a.IsCoreBusy()).Clicked() {
				a.CheckQuality()
			}
			if components.ActionButton(c.Key("quality-close"), p, s, nil, a.Tr("关闭", "Close")).Clicked() {
				a.ShowQualityModal = false
			}
		})
	})
}
