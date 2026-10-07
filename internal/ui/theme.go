package ui

import (
	"embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The fonts are SIL OFL; their licence text sits beside them in fonts/.
//
//go:embed fonts/AtkinsonHyperlegibleNext-Regular.ttf fonts/AtkinsonHyperlegibleNext-Bold.ttf fonts/AtkinsonHyperlegibleMono-Regular.ttf
var fontFS embed.FS

var (
	fontText = mustFont("AtkinsonHyperlegibleNext-Regular.ttf")
	fontBold = mustFont("AtkinsonHyperlegibleNext-Bold.ttf")
	fontMono = mustFont("AtkinsonHyperlegibleMono-Regular.ttf")
)

func mustFont(name string) fyne.Resource {
	b, err := fontFS.ReadFile("fonts/" + name)
	if err != nil {
		panic(err)
	}
	return fyne.NewStaticResource(name, b)
}

type palette struct {
	bg, surface, line, fg, muted, pink, ok, errc, warn color.NRGBA
}

var (
	darkPalette = palette{
		bg: rgb(0x19171a), surface: rgb(0x232024), line: rgb(0x36313a),
		fg: rgb(0xf2edf0), muted: rgb(0xa3979e), pink: rgb(0xf02460),
		ok: rgb(0x7fd49a), errc: rgb(0xff7a7a), warn: rgb(0xe8b04a),
	}
	lightPalette = palette{
		bg: rgb(0xfaf7f8), surface: rgb(0xf0eaed), line: rgb(0xddd3d8),
		fg: rgb(0x211a1e), muted: rgb(0x6c5f66), pink: rgb(0xdc124b),
		ok: rgb(0x237a43), errc: rgb(0xb3261e), warn: rgb(0x8a5a00),
	}
)

var (
	screenBlack = rgb(0x0d0b0c)
	screenCell  = rgb(0x2a2528)
	screenMuted = rgb(0xa3979e)
	white       = rgb(0xffffff)
)

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

func paletteFor(v fyne.ThemeVariant) palette {
	if v == theme.VariantLight {
		return lightPalette
	}
	return darkPalette
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// moanTheme follows the system light/dark variant; the palette is the
// moansubs.org design tokens.
type moanTheme struct{}

var _ fyne.Theme = moanTheme{}

func (moanTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	p := paletteFor(v)
	switch n {
	case theme.ColorNameBackground:
		return p.bg
	case theme.ColorNameInputBackground, theme.ColorNameOverlayBackground,
		theme.ColorNameMenuBackground, theme.ColorNameButton, theme.ColorNameHeaderBackground:
		return p.surface
	case theme.ColorNameSeparator, theme.ColorNameInputBorder,
		theme.ColorNameScrollBar:
		return p.line
	case theme.ColorNameForeground:
		return p.fg
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return p.muted
	case theme.ColorNameDisabledButton:
		return p.surface
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return p.pink
	case theme.ColorNameSelection:
		return withAlpha(p.pink, 0x55)
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnError,
		theme.ColorNameForegroundOnSuccess:
		return white
	case theme.ColorNameForegroundOnWarning:
		return rgb(0x1a1204)
	case theme.ColorNameSuccess:
		return p.ok
	case theme.ColorNameError:
		return p.errc
	case theme.ColorNameWarning:
		return p.warn
	case theme.ColorNameHover:
		return withAlpha(p.fg, 0x14)
	case theme.ColorNamePressed:
		return withAlpha(p.fg, 0x26)
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x40}
	}
	return theme.DefaultTheme().Color(n, v)
}

// Font has no italic face, so italic falls back to upright.
func (moanTheme) Font(s fyne.TextStyle) fyne.Resource {
	switch {
	case s.Symbol:
		return theme.DefaultTheme().Font(s)
	case s.Monospace:
		return fontMono
	case s.Bold:
		return fontBold
	}
	return fontText
}

func (moanTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (moanTheme) Size(n fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(n)
}
