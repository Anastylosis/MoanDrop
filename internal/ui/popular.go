package ui

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// showPopular opens the "What's popular" list. Rows are informational:
// they are releases on the node, with no local file to attach a subtitle
// to, so the only action is the link to the release's page.
func (u *appUI) showPopular() {
	u.withFeature(core.FeatureTrending, func(supported bool) {
		if !supported {
			dialog.ShowInformation("What's popular", core.TrendingUnsupportedMessage, u.win)
			return
		}
		body := container.NewVBox(widget.NewLabel("loading..."))
		scroll := container.NewVScroll(body)
		scroll.SetMinSize(fyne.NewSize(480, 360))
		dialog.NewCustom("What's popular", "Close", scroll, u.win).Show()

		server := serverURL(u.app.Preferences())
		go func() {
			items, err := client.New(server, "").Trending(context.Background(), 0, 0)
			fyne.Do(func() {
				body.RemoveAll()
				if err != nil {
					body.Add(wrappedLabel(core.ExplainError(err).Error()))
				} else {
					u.fillPopular(body, server, items)
				}
				body.Refresh()
			})
		}()
	})
}

func (u *appUI) fillPopular(body *fyne.Container, server string, items []client.TrendingRelease) {
	if len(items) == 0 {
		body.Add(wrappedLabel(core.TrendingEmptyMessage))
		return
	}
	body.Add(widget.NewLabelWithStyle(core.TrendingHeading(0), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	body.Add(wrappedLabel(core.TrendingNoFileNote))
	body.Add(widget.NewSeparator())
	for _, row := range core.TrendingRows(items) {
		title := widget.NewLabelWithStyle(row.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		title.Wrapping = fyne.TextWrapWord
		entry := container.NewVBox(title, wrappedLabel(row.Detail()))
		if link, err := core.ReleaseLink(server, row.ReleaseID); err == nil {
			entry.Add(widget.NewHyperlink("Open release page", link))
		}
		body.Add(entry)
		body.Add(widget.NewSeparator())
	}
}

// showNodeTotals is the Help menu's "About this node" entry.
func (u *appUI) showNodeTotals() {
	server := serverURL(u.app.Preferences())
	body := container.NewVBox(monoLabel(server))
	u.withFeature(core.FeatureStats, func(supported bool) {
		if !supported {
			body.Add(wrappedLabel(core.StatsUnsupportedMessage))
			dialog.NewCustom("About this node", "Close", body, u.win).Show()
			return
		}
		totals := wrappedLabel("loading...")
		body.Add(totals)
		dialog.NewCustom("About this node", "Close", body, u.win).Show()
		go func() {
			s, err := client.New(server, "").Stats(context.Background())
			fyne.Do(func() {
				if err != nil {
					totals.SetText(core.ExplainError(err).Error())
					return
				}
				totals.SetText(strings.Join(core.StatsLines(s), "\n"))
			})
		}()
	})
}

func wrappedLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}
