package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// sceneForm is the optional title/date/studio/performers block shared by the
// share dialog and the standalone "Add scene details" dialog.
type sceneForm struct {
	title, date, studio, performers *widget.Entry
}

func newSceneForm() *sceneForm {
	f := &sceneForm{title: widget.NewEntry(), date: widget.NewEntry(), studio: widget.NewEntry(), performers: widget.NewEntry()}
	f.title.SetPlaceHolder("title")
	f.date.SetPlaceHolder(core.DateHintText)
	f.studio.SetPlaceHolder("studio")
	f.performers.SetPlaceHolder(core.PerformersPlaceholder)
	return f
}

func (f *sceneForm) objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{f.title, f.date, f.studio, f.performers}
}

func (f *sceneForm) details() core.SceneDetails {
	return core.SceneDetails{
		Title: f.title.Text, Date: f.date.Text, Studio: f.studio.Text,
		Performers: core.ParsePerformers(f.performers.Text),
	}.Trim()
}

// askSceneDetails is the standalone dialog behind "Add scene details";
// onSubmit is never called on cancel, an empty form or a malformed date.
func askSceneDetails(win fyne.Window, onSubmit func(core.SceneDetails)) {
	form := newSceneForm()
	help := widget.NewLabel(core.SceneDetailsHelp)
	help.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(append([]fyne.CanvasObject{help}, form.objects()...)...)
	dialog.NewCustomConfirm(core.AddSceneDetailsLabel, "Send", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		d := form.details()
		if d.Empty() {
			return
		}
		if err := d.Validate(); err != nil {
			showError(win, err)
			return
		}
		onSubmit(d)
	}, win).Show()
}

// offerDetails adds the "Add scene details" action to a result card when the
// release has no title and the file is byte-identical to it: the node
// resolves details by oshash, so a merely similar encode can't be named.
// The returned probe runs after the caller finishes laying out, so a cached
// answer can't mutate widgets mid-render.
func (u *appUI) offerDetails(card *fyne.Container, r client.Release) (probe func()) {
	slot := container.NewHBox()
	card.Add(slot)
	return func() {
		u.withFeature(core.FeatureMetadata, func(supported bool) {
			if !supported {
				return
			}
			slot.Add(widget.NewButton(core.AddSceneDetailsLabel, func() { u.addSceneDetails(r.OSHash) }))
			slot.Refresh()
		})
	}
}

func (u *appUI) addSceneDetails(oshash string) {
	if token(u.app.Preferences()) == "" {
		u.promptToken(func() { u.addSceneDetails(oshash) })
		return
	}
	askSceneDetails(u.win, func(d core.SceneDetails) {
		if u.detailsBusy {
			return
		}
		u.detailsBusy = true
		gen := u.matchGen
		c := client.New(serverURL(u.app.Preferences()), token(u.app.Preferences()))
		go func() {
			res, err := core.ContributeDetailsFor(context.Background(), c, oshash, d)
			fyne.Do(func() {
				u.detailsBusy = false
				if gen != u.matchGen {
					return
				}
				if err != nil {
					showError(u.win, err)
					return
				}
				u.setStatus(res.Message())
			})
		}()
	})
}
