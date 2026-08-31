package core

import "testing"

func TestResolveCaptionLang(t *testing.T) {
	cases := []struct {
		in         string
		base       string
		normalized bool
		wantErr    bool
	}{
		{"en", "en", false, false},
		{"pt-BR", "pt", true, false},
		{"pt", "pt", false, false},
		{"zh-Hant", "zh", true, false},
		{"", "", false, true},
		{"not a tag", "", false, true},
	}
	for _, c := range cases {
		got, err := ResolveCaptionLang(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ResolveCaptionLang(%q): want error, got %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ResolveCaptionLang(%q): %v", c.in, err)
			continue
		}
		if got.Base != c.base || got.Normalized != c.normalized || got.Original != c.in {
			t.Errorf("ResolveCaptionLang(%q) = %+v, want base %q normalized %v", c.in, got, c.base, c.normalized)
		}
	}
}

func TestInferSidecarLang(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/videos/Scene.Title.en.srt", "en"},
		{"/videos/Scene.Title.pt-BR.srt", "pt-BR"},
		{"/videos/Scene.Title.srt", ""},       // no tag segment
		{"/videos/Scene.Title.2019.srt", ""},  // year, not a language
		{"/videos/Scene.Title.1080p.srt", ""}, // resolution, not a language
		{"C:\\videos\\Scene.de.srt", "de"},    // backslashes are not separators off Windows, but the suffix still parses
		{"/videos/subtitle.vtt", ""},          // bare stem
	}
	for _, c := range cases {
		if got := InferSidecarLang(c.path); got != c.want {
			t.Errorf("InferSidecarLang(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
