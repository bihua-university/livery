package app

import (
	"context"
	"net/url"
	"time"

	"github.com/bihua-university/livery/internal/diagnostics"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/profile"
)

func (a *App) QualityChecking() bool { return a.qualityCancel != nil }

func (a *App) resetQuality() {
	if a.qualityCancel != nil {
		a.qualityCancel()
		a.qualityCancel = nil
	}
	a.QualityReport = diagnostics.Report{}
}

func (a *App) CheckQuality() {
	if a.QualityChecking() || a.CoreInfo.Status != model.CoreRunning || a.IsCoreBusy() {
		return
	}
	port := a.SettingsData.MixedPort
	client, info := a.APIClient(), a.CoreInfo
	profileID := a.SettingsData.ActiveProfileID
	ctx, cancel := context.WithCancel(context.Background())
	a.qualityCancel = cancel
	a.QualityReport = diagnostics.Report{}
	go func() {
		run, stop := context.WithTimeout(ctx, 14*time.Second)
		defer stop()
		localIP := detectLocalIPv4()
		var proxy *url.URL
		var err error
		if info.CoreType == model.CoreSingBox && !info.External {
			proxy, err = profile.SingBoxProxyURL(info.RuntimePath)
		} else {
			proxy, err = client.ProxyURL(run, info.CoreType, port)
		}
		report := diagnostics.Report{CheckedAt: time.Now(), IPError: err}
		if err == nil {
			report = a.qualityChecker.Check(run, proxy)
		}
		networkChanged := localIP != detectLocalIPv4()
		if ctx.Err() != nil || a.shuttingDown.Load() {
			return
		}
		a.UpdateUI(func() {
			if ctx.Err() != nil {
				return
			}
			a.resetQuality()
			if a.shuttingDown.Load() || !a.ShowQualityModal || a.CoreInfo.Status != model.CoreRunning || a.IsCoreBusy() || a.SettingsData.MixedPort != port || a.SettingsData.ActiveProfileID != profileID {
				return
			}
			if networkChanged {
				a.Notify(a.Tr("网络已切换，请重新检测", "Network changed. Run the check again."))
				return
			}
			a.QualityReport = report
		})
	}()
}
