// Package i18n renders the server-side error message for an API error
// envelope, keyed by error code and selected by the request's
// Accept-Language header. See libs/i18n/README.md for why this catalog is
// separate from the frontend's, and docs/audit-and-errors.md §1a for the
// three-planes rule this package is one leg of: the request locale affects
// the rendered message only, never storage.
package i18n

import (
	"embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed locales/*.json
var localeFiles embed.FS

// SourceLocale is the catalog every error code must have an entry in, and
// the fallback when a request's Accept-Language matches nothing this server
// ships (docs/audit-and-errors.md, libs/i18n/README.md).
const SourceLocale = "en-US"

type catalog struct {
	Errors map[string]string `json:"errors"`
}

var catalogs = mustLoadCatalogs()

func mustLoadCatalogs() map[string]catalog {
	entries, err := localeFiles.ReadDir("locales")
	if err != nil {
		panic("i18n: read locales: " + err.Error())
	}

	out := make(map[string]catalog, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		locale := strings.TrimSuffix(name, ".json")

		raw, err := localeFiles.ReadFile("locales/" + name)
		if err != nil {
			panic("i18n: read " + name + ": " + err.Error())
		}

		var c catalog
		if err := json.Unmarshal(raw, &c); err != nil {
			panic("i18n: parse " + name + ": " + err.Error())
		}
		out[locale] = c
	}

	if _, ok := out[SourceLocale]; !ok {
		panic("i18n: missing source locale catalog " + SourceLocale + ".json")
	}

	return out
}

// Render returns the message for code in the given Accept-Language, falling
// back to SourceLocale when the header names no locale this server ships, or
// when the matched locale's catalog has no entry for code (which
// scripts/check_i18n_parity.py should never allow to happen, but Render does
// not trust that at runtime).
func Render(code string, acceptLanguage string) string {
	locale := ResolveLocale(acceptLanguage)

	if msg, ok := catalogs[locale].Errors[code]; ok {
		return msg
	}
	return catalogs[SourceLocale].Errors[code]
}

// ResolveLocale picks the best supported locale from an Accept-Language
// header, matching by exact tag first (vi-VN) then by base language (vi),
// in the header's own preference order. It never trusts the header's q
// weighting beyond that ordering — a malformed header degrades to
// SourceLocale rather than erroring. Exported so a middleware can resolve
// once per request and store the result, rather than every call site
// re-parsing the header.
func ResolveLocale(acceptLanguage string) string {
	for _, tag := range parsePreferenceOrder(acceptLanguage) {
		if _, ok := catalogs[tag]; ok {
			return tag
		}
		base, _, _ := strings.Cut(tag, "-")
		for locale := range catalogs {
			if strings.EqualFold(strings.SplitN(locale, "-", 2)[0], base) {
				return locale
			}
		}
	}
	return SourceLocale
}

type weightedTag struct {
	tag    string
	weight float64
}

// parsePreferenceOrder turns "vi-VN,vi;q=0.9,en-US;q=0.8" into
// ["vi-VN", "vi", "en-US"], highest weight first, ties kept in header order.
func parsePreferenceOrder(header string) []string {
	if header == "" {
		return nil
	}

	var tags []weightedTag
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		tag, params, _ := strings.Cut(part, ";")
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == "*" {
			continue
		}

		weight := 1.0
		if q, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if parsed, err := strconv.ParseFloat(q, 64); err == nil {
				weight = parsed
			}
		}

		tags = append(tags, weightedTag{tag: tag, weight: weight})
	}

	// Stable sort by descending weight, keeping header order for ties.
	for i := 1; i < len(tags); i++ {
		for j := i; j > 0 && tags[j].weight > tags[j-1].weight; j-- {
			tags[j], tags[j-1] = tags[j-1], tags[j]
		}
	}

	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.tag
	}
	return out
}
