package i18n

import "testing"

func TestRender_KnownCodeInSourceLocale(t *testing.T) {
	got := Render("VALIDATION_FAILED", "en-US")
	want := "The request contains invalid fields."
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRender_KnownCodeInSecondLocale(t *testing.T) {
	got := Render("VALIDATION_FAILED", "vi-VN")
	if got == "" {
		t.Fatal("Render returned empty string for a declared vi-VN entry")
	}
	if got == Render("VALIDATION_FAILED", "en-US") {
		t.Error("vi-VN message equals en-US message; catalogs should differ per language")
	}
}

func TestRender_UnsupportedLocaleFallsBackToSource(t *testing.T) {
	got := Render("VALIDATION_FAILED", "de-DE")
	want := Render("VALIDATION_FAILED", SourceLocale)
	if got != want {
		t.Errorf("Render(de-DE) = %q, want source fallback %q", got, want)
	}
}

func TestRender_EmptyAcceptLanguageFallsBackToSource(t *testing.T) {
	got := Render("VALIDATION_FAILED", "")
	want := Render("VALIDATION_FAILED", SourceLocale)
	if got != want {
		t.Errorf("Render('') = %q, want source fallback %q", got, want)
	}
}

func TestRender_MatchesBaseLanguageWithoutRegion(t *testing.T) {
	got := Render("VALIDATION_FAILED", "vi")
	want := Render("VALIDATION_FAILED", "vi-VN")
	if got != want {
		t.Errorf("Render(vi) = %q, want %q (base-language match)", got, want)
	}
}

func TestRender_RespectsQWeightOverListOrder(t *testing.T) {
	// vi-VN listed first but weighted lower than en-US: en-US must win.
	got := Render("VALIDATION_FAILED", "vi-VN;q=0.5, en-US;q=0.9")
	want := Render("VALIDATION_FAILED", "en-US")
	if got != want {
		t.Errorf("Render with q-weights = %q, want %q", got, want)
	}
}

func TestRender_UnknownCodeReturnsEmptyString(t *testing.T) {
	got := Render("NOT_A_REAL_CODE", "en-US")
	if got != "" {
		t.Errorf("Render(unknown code) = %q, want empty string", got)
	}
}

func TestRender_CodeIdenticalAcrossLocalesOnlyMessageDiffers(t *testing.T) {
	// The three-planes rule as a test: the locale changes the message, never
	// the code the caller branches on. Render only returns the message, so
	// this asserts the caller-facing contract at the boundary that matters:
	// a code present in one locale is present in every locale.
	for code := range catalogs[SourceLocale].Errors {
		for locale := range catalogs {
			if _, ok := catalogs[locale].Errors[code]; !ok {
				t.Errorf("code %q missing from locale %q catalog", code, locale)
			}
		}
	}
}
