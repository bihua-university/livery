package pages

import (
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Profiles renders the subscription & local configuration management page.
func Profiles(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Column(c).Fill().Padding(16).Gap(12).MinHeight(0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			if components.PrimaryActionButton(c, p, s, icons.Download, a.Tr("导入订阅链接", "Import URL")).Clicked() {
				a.ImportURL = ""
				a.ImportName = ""
				a.ImportUA = ""
				a.ShowImportModal = true
			}
			if components.ActionButton(c, p, s, icons.Folder, a.Tr("导入本地文件", "Import File")).Clicked() {
				go a.PickLocalProfile()
			}
			if components.ActionButton(c, p, s, icons.Plus, a.Tr("新建空白配置", "New Blank")).Clicked() {
				a.NewProfileName = ""
				a.ShowNewProfileModal = true
			}
			ui.Spacer(c)
			if components.ActionButton(c, p, s, icons.Code, a.Tr("查看运行配置", "Runtime Config")).Clicked() {
				a.OpenRuntimePreview()
			}
			if components.ActionButton(c, p, s, icons.Refresh, a.Tr("全部更新", "Update All")).Clicked() {
				a.RefreshAllProfilesAsync()
			}
		})

		ui.Scroll(c.Key("profiles-scroll")).TrackScroll(&a.ProfilesScroll).Grow(1).MinHeight(0).Children(func() {
			ui.Grid(c).Columns(2).Gap(12).Children(func() {
				for _, prof := range a.Profiles {
					profileCard(c.Key("prof-"+prof.ID), a, p, s, prof)
				}
			})
		})
	})
}

func profileCard(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale, prof model.Profile) {
	active := prof.ID == a.SettingsData.ActiveProfileID
	box := ui.Column(c).
		Background(p.Card).
		Border(1, p.CardBorder).
		Radius(theme.RadiusXL).
		Padding(16).
		Gap(10)
	if active {
		box.BorderColor(p.Primary).Background(p.CardSelected)
	}
	box.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, prof.Name).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground).Grow(1).SingleLine()
			if active {
				components.StatusChip(c, s, a.Tr("使用中", "Active"), p.SuccessSurface, p.SuccessSurfaceForeground, p.Success)
			}
			kindLabel := a.Tr("本地", "Local")
			if prof.Kind == "remote" {
				kindLabel = a.Tr("订阅", "Remote")
			}
			components.TagChip(c, p, s, kindLabel)
		})

		if prof.Kind == "remote" {
			ui.Text(c, prof.URL).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle).SingleLine()
		} else {
			ui.Text(c, prof.FileName).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle).SingleLine()
		}

		components.QuotaBar(c, p, s, prof.Usage, "")

		if prof.LastError != "" {
			ui.Box(c).Background(p.DestructiveSurface).Radius(theme.RadiusMD).Padding(6, 8).Children(func() {
				ui.Text(c, prof.LastError).FontSize(s.XS).TextColor(p.DestructiveSurfaceFg).MaxLines(2)
			})
		}

		ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, a.Tr("更新于 ", "Updated ")+components.FormatRelativeTime(prof.UpdatedAt, a.SettingsData.Language)).
				FontSize(s.XS).TextColor(p.ForegroundSubtlest).Grow(1)

			if !active {
				id := prof.ID
				components.PrimaryActionButton(c, p, s, icons.Check, a.Tr("启用", "Use")).OnClick(func() {
					a.ActivateProfile(id)
				})
			}
			if prof.Kind == "remote" {
				id := prof.ID
				components.IconActionButton(c, p, icons.Refresh, a.Tr("更新订阅", "Refresh")).OnClick(func() {
					a.RefreshProfileAsync(id)
				})
			}
			id := prof.ID
			components.IconActionButton(c, p, icons.Code, a.Tr("编辑配置", "Edit Config")).OnClick(func() {
				a.OpenProfileEditor(id)
			})
			profCopy := prof
			components.IconActionButton(c, p, icons.Sliders, a.Tr("覆写与属性", "Overrides & Info")).OnClick(func() {
				a.MetaDraft = profCopy
				a.ShowMetaModal = true
			})
			components.IconActionButton(c, p, icons.Copy, a.Tr("复制一份", "Duplicate")).OnClick(func() {
				if _, err := a.PM.Duplicate(id); err != nil {
					a.Notify(err.Error())
				} else {
					a.Profiles = a.PM.Profiles()
					a.Notify(a.Tr("已复制配置", "Profile duplicated"))
				}
			})
			if len(a.Profiles) > 1 {
				components.IconActionButton(c, p, icons.Trash, a.Tr("删除", "Delete")).OnClick(func() {
					a.DeleteTargetID = id
					a.ShowDeleteAlert = true
				})
			}
		})
	})
}
