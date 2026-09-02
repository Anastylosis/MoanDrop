package ui

import (
	"context"
	"slices"

	"fyne.io/fyne/v2"

	"github.com/Anastylosis/MoanSubs/client"
)

// withFeature answers whether the configured node advertises name in GET
// /api/v1/version's feature list, probing once per run and caching the
// answer — so a node that lacks a feature is never sent a request it would
// 404, and the question costs one round trip per run, not one per click.
// fn runs on the UI goroutine: synchronously when the answer is cached,
// else after the probe's fyne.Do. A probe that outlives the video it was
// started for (matchGen moved on) drops fn, the same discipline every
// other async continuation here follows; a failed probe reads as "nothing
// advertised" rather than an error, since every caller has a plain path.
func (u *appUI) withFeature(name string, fn func(supported bool)) {
	if u.featuresProbed {
		fn(slices.Contains(u.features, name))
		return
	}
	server := serverURL(u.app.Preferences())
	gen := u.matchGen
	go func() {
		v, err := client.New(server, "").Version(context.Background())
		var features []string
		if err == nil {
			features = v.Features
		}
		fyne.Do(func() {
			u.features = features
			u.featuresProbed = true
			if gen == u.matchGen {
				fn(slices.Contains(features, name))
			}
		})
	}()
}
