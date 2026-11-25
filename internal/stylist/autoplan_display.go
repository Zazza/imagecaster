package stylist

import (
	"encoding/json"
	"strings"
	"unicode"
)

const (
	suggestDisplayBatchBytes = 1600
	suggestDisplayWarning    = "Не удалось перевести часть описаний: показан английский текст"
)

func suggestPlaceholder(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "auto", "keep", "unspecified", "...", "…", "short title", "one places value", "one options id", "selected option id", "english photographic light, composition and focus instructions", "english pose and composition", "english wardrobe description", "english hairstyle description", "english makeup description", "english scene description", "english outfit description", "english background description", "english description", "english ... description":
		return true
	}
	return false
}

func suggestRussian(v string) bool {
	return strings.ContainsFunc(v, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) && !cjkRe.MatchString(v) && !suggestPlaceholder(v)
}

func localizeSuggest(chat Ideas, model, locale string, plan *SuggestResult) bool {
	if locale != "ru" {
		return false
	}
	var sources []string
	var destinations []*string
	add := func(source string, destination *string) {
		if suggestRussian(*destination) {
			return
		}
		*destination = ""
		if strings.TrimSpace(source) == "" {
			return
		}
		if suggestRussian(source) {
			*destination = source
			return
		}
		sources = append(sources, source)
		destinations = append(destinations, destination)
	}
	add(plan.TargetReason, &plan.TargetReasonRu)
	add(plan.Concept, &plan.ConceptRu)
	add(plan.Light, &plan.LightRu)
	add(plan.Focus, &plan.FocusRu)
	for i := range plan.Series {
		s := &plan.Series[i]
		add(s.Direction, &s.DirectionRu)
		for _, a := range seriesAspects {
			add(*seriesField(s, a), seriesRuField(s, a))
		}
		for j := range s.Shots {
			add(s.Shots[j].Prompt, &s.Shots[j].PromptRu)
		}
	}
	failed := false
	for start := 0; start < len(sources); {
		end := start
		for end < len(sources) {
			encoded, err := json.Marshal(map[string][]string{"texts": sources[start : end+1]})
			if err != nil || len(encoded) > suggestDisplayBatchBytes {
				break
			}
			end++
		}
		if end == start {
			failed = true
			start++
			continue
		}
		translated, err := TranslateToRu(chat, model, sources[start:end])
		if err != nil {
			failed = true
		} else {
			for i, text := range translated {
				if suggestRussian(text) {
					*destinations[start+i] = text
				} else {
					failed = true
				}
			}
		}
		start = end
	}
	return failed
}
