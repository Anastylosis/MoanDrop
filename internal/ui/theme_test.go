package ui

import (
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/Anastylosis/MoanSubs/client"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

func TestMoanTheme_ColorsPerVariant(t *testing.T) {
	cases := []struct {
		name    fyne.ThemeColorName
		variant fyne.ThemeVariant
		want    color.NRGBA
	}{
		{theme.ColorNameBackground, theme.VariantDark, rgb(0x19171a)},
		{theme.ColorNameBackground, theme.VariantLight, rgb(0xfaf7f8)},
		{theme.ColorNameInputBackground, theme.VariantDark, rgb(0x232024)},
		{theme.ColorNameInputBackground, theme.VariantLight, rgb(0xf0eaed)},
		{theme.ColorNameSeparator, theme.VariantDark, rgb(0x36313a)},
		{theme.ColorNameSeparator, theme.VariantLight, rgb(0xddd3d8)},
		{theme.ColorNameForeground, theme.VariantDark, rgb(0xf2edf0)},
		{theme.ColorNameForeground, theme.VariantLight, rgb(0x211a1e)},
		{theme.ColorNamePlaceHolder, theme.VariantDark, rgb(0xa3979e)},
		{theme.ColorNameDisabled, theme.VariantLight, rgb(0x6c5f66)},
		{theme.ColorNamePrimary, theme.VariantDark, rgb(0xf02460)},
		{theme.ColorNamePrimary, theme.VariantLight, rgb(0xdc124b)},
		{theme.ColorNameFocus, theme.VariantDark, rgb(0xf02460)},
		{theme.ColorNameHyperlink, theme.VariantLight, rgb(0xdc124b)},
		{theme.ColorNameForegroundOnPrimary, theme.VariantDark, rgb(0xffffff)},
		{theme.ColorNameSuccess, theme.VariantDark, rgb(0x7fd49a)},
		{theme.ColorNameSuccess, theme.VariantLight, rgb(0x237a43)},
	}
	for _, c := range cases {
		if got := (moanTheme{}).Color(c.name, c.variant); got != c.want {
			t.Errorf("%s variant %v = %v, want %v", c.name, c.variant, got, c.want)
		}
	}
}

func TestMoanTheme_ErrorContrast(t *testing.T) {
	for _, v := range []fyne.ThemeVariant{theme.VariantDark, theme.VariantLight} {
		th := moanTheme{}
		if r := contrast(th.Color(theme.ColorNameError, v), th.Color(theme.ColorNameBackground, v)); r < 4.5 {
			t.Errorf("error on background contrast %.2f in variant %v, want >= 4.5", r, v)
		}
	}
}

func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		f := float64(v) / 0xffff
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func TestMoanTheme_Fonts(t *testing.T) {
	th := moanTheme{}
	if th.Font(fyne.TextStyle{}) != fontText || th.Font(fyne.TextStyle{Bold: true}) != fontBold ||
		th.Font(fyne.TextStyle{Monospace: true}) != fontMono {
		t.Error("theme does not serve the bundled Atkinson faces")
	}
}

func TestFrameProgress_Advances(t *testing.T) {
	test.NewApp()
	f := newFrameProgress(25)
	w := test.NewWindow(f)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 60))

	f.SetDone(7)
	if f.Done() != 7 {
		t.Fatalf("Done = %d, want 7", f.Done())
	}
	r := test.WidgetRenderer(f).(*frameRenderer)
	on := 0
	for _, c := range r.cells {
		if c.FillColor != screenCell {
			on++
		}
	}
	if on != 7 {
		t.Errorf("%d cells filled, want 7", on)
	}
	f.SetDone(99)
	if f.Done() != 25 {
		t.Errorf("Done = %d after overshoot, want clamp to 25", f.Done())
	}
}

func TestDropZone_LineTracksFrames(t *testing.T) {
	test.NewApp()
	z := newDropZone()
	if z.line.Text != dropHint || z.cells.Done() != 0 {
		t.Errorf("idle zone shows %q with %d cells", z.line.Text, z.cells.Done())
	}
	z.SetFrames(18)
	if z.line.Text != "Fingerprinting: 18 of 25 frames sampled" {
		t.Errorf("line = %q", z.line.Text)
	}
	z.SetFrames(-1)
	if z.line.Text != dropHint || z.cells.Done() != 0 {
		t.Error("SetFrames(-1) did not return to idle")
	}
}

// forcedVariant pins moanTheme to one variant, since the test app has no system setting.
type forcedVariant struct {
	moanTheme
	v fyne.ThemeVariant
}

func (f forcedVariant) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return f.moanTheme.Color(n, f.v)
}

// TestRenderScreenshots writes dark and light PNGs of the window with sample
// data when MOANDROP_SHOT_DIR is set; it asserts nothing otherwise.
func TestRenderScreenshots(t *testing.T) {
	dir := os.Getenv("MOANDROP_SHOT_DIR")
	if dir == "" {
		t.Skip("MOANDROP_SHOT_DIR not set")
	}
	for name, v := range map[string]fyne.ThemeVariant{"dark": theme.VariantDark, "light": theme.VariantLight} {
		a := test.NewApp()
		a.Settings().SetTheme(forcedVariant{v: v})
		setToken(a.Preferences(), "tok")
		u := newTestApp(a)
		u.drop.SetFrames(18)
		release := client.Release{ID: 214, Title: "Late.Checkout.Room.214.1080p.mp4"}
		u.renderCandidates([]CandidateRow{{
			Release:    release,
			Confidence: core.ConfidenceExact,
			Evidence:   "byte-identical file",
			Tracks: []TrackRow{
				{Track: client.TrackSummary{ID: 1, Lang: "en", Kind: "sdh", Revision: 2, Downloads: 412, Up: 38, Down: 2}, Credit: "by mira_k"},
				{Track: client.TrackSummary{ID: 2, Lang: "de", Generated: true, Downloads: 131, Up: 6, Down: 1}, Badge: core.LabelDeclaredGenerated, Tooltip: core.GeneratedExplainer},
			},
		}})
		u.setStatus("found 1 candidate(s)")
		u.win.Resize(fyne.NewSize(900, 560))
		img := u.win.Canvas().Capture()
		f, err := os.Create(filepath.Join(dir, "moandrop-"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
}
