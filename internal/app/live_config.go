package app

import (
	"context"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/clashapi"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/sysproxy"
)

// ApplyProfileChanges reloads the active profile in a managed Mihomo process.
func (a *App) ApplyProfileChanges() {
	if a.CoreInfo.Status != model.CoreRunning {
		return
	}
	if a.CoreInfo.External {
		a.Notify(a.Tr("已保存，下次由 Livery 启动核心时生效", "Saved for the next Livery-managed start"))
		return
	}
	if a.CoreInfo.CoreType != model.CoreMihomo {
		go a.RestartCore()
		return
	}
	client, session, id := a.APIClient(), a.CoreInfo.StartedAt, a.SettingsData.ActiveProfileID
	go func() {
		if err := a.reloadProfile(client, session, id); err != nil {
			a.UpdateUI(func() {
				a.Notify(a.Tr("配置已保存，热重载失败：", "Configuration saved; live reload failed: ") + err.Error())
			})
		}
	}()
}

func (a *App) reloadProfile(client *clashapi.Client, session time.Time, profileID string) error {
	a.coreOpMu.Lock()
	defer a.coreOpMu.Unlock()
	info := a.CM.Info()
	if a.shuttingDown.Load() || info.External || info.Status != model.CoreRunning || info.StartedAt != session || a.PM.Settings().ActiveProfileID != profileID {
		return nil
	}
	payload, err := a.PM.RenderActive(a.PM.Settings())
	if err != nil {
		return err
	}
	if err := client.ReloadConfig(context.Background(), payload); err != nil {
		return err
	}
	a.recordLiveConfig(payload)
	return nil
}

// ApplyInbound commits the new listener setting once the running Mihomo
// controller confirms it. In particular, Proxy Guard must not see a new mixed
// port before the listener has moved.
func (a *App) ApplyInbound(field string) {
	want := a.SettingsData
	if a.CoreInfo.Status != model.CoreRunning || model.NormalizeCoreType(want.CoreType) != model.CoreMihomo {
		a.ApplySettings(true)
		if field == "mixed-port" && want.SystemProxyEnabled {
			go a.syncMixedPort()
		}
		return
	}
	old := a.PM.Settings()
	var value any
	switch field {
	case "mixed-port":
		value = want.MixedPort
		a.SettingsData.MixedPort = old.MixedPort
	case "allow-lan":
		value = want.AllowLAN
		a.SettingsData.AllowLAN = old.AllowLAN
	default:
		return
	}
	if a.ApplyingInbound {
		a.Notify(a.Tr("正在应用入站设置，请稍后重试", "Applying inbound settings. Try again shortly."))
		return
	}
	a.ApplyingInbound = true
	session, client := a.CoreInfo.StartedAt, a.APIClient()
	go func() {
		err := a.patchInbound(client, session, want, field, value)
		a.UpdateUI(func() {
			a.ApplyingInbound = false
			if err != nil {
				if field == "mixed-port" {
					a.MixedPortDraft = fmt.Sprint(a.SettingsData.MixedPort)
				}
				a.Notify(a.Tr("入站设置热更新失败：", "Live inbound update failed: ") + err.Error())
				return
			}
			if field == "allow-lan" {
				a.SettingsData.AllowLAN = want.AllowLAN
			} else {
				a.SettingsData.MixedPort = want.MixedPort
			}
			if field == "mixed-port" && a.SettingsData.SystemProxyEnabled {
				go a.syncMixedPort()
			}
			a.Notify(a.Tr("入站设置已生效", "Inbound settings applied"))
		})
	}()
}

func (a *App) patchInbound(client *clashapi.Client, session time.Time, want model.AppSettings, field string, value any) error {
	a.coreOpMu.Lock()
	defer a.coreOpMu.Unlock()
	info := a.CM.Info()
	if a.shuttingDown.Load() || info.Status != model.CoreRunning || info.StartedAt != session {
		return fmt.Errorf("core session changed")
	}
	var payload []byte
	if !info.External {
		s := a.PM.Settings()
		if field == "mixed-port" {
			s.MixedPort = want.MixedPort
		} else {
			s.AllowLAN = want.AllowLAN
		}
		raw, err := a.PM.RenderActive(s)
		if err != nil {
			return err
		}
		var cfg map[string]any
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return err
		}
		if cfg[field] != value {
			return fmt.Errorf("active profile controls %s", field)
		}
		payload = raw
	}
	if err := client.PatchInbound(context.Background(), field, value); err != nil {
		return err
	}
	if payload != nil {
		a.recordLiveConfig(payload)
	}
	return a.PM.SaveInboundSetting(field, want)
}

func (a *App) recordLiveConfig(payload []byte) {
	if _, err := a.PM.SaveRuntimeConfig(payload); err != nil {
		a.LogApp("warning", "Could not write the applied runtime configuration: "+err.Error())
	}
}

func (a *App) syncMixedPort() {
	a.sysproxyMu.Lock()
	defer a.sysproxyMu.Unlock()
	s := a.PM.Settings()
	if s.SystemProxyEnabled && !a.shuttingDown.Load() {
		if err := sysproxy.Enable("127.0.0.1", effectiveMixedPort(s), s.SystemProxyBypass); err != nil {
			a.UpdateUI(func() { a.Notify(a.Tr("系统代理更新失败：", "System proxy update failed: ") + err.Error()) })
		}
	}
}
