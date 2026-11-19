package stylist

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Zazza/imagecaster/internal/promptutil"
)

const (
	TranslateMax          = 64
	translateInMaxLen     = 1000
	translateTemperature  = 0.2
	translateMaxTokens    = 4000
	translateQuoteMaxRune = 60
)

const translateSystemPrompt = `Ты — переводчик для генерации изображений. Переведи каждый русский текст плана фотосессии на английский для промпта модели изображений. Пиши только по-английски, коротко, фразами через запятую. Сохрани все детали: вид вещи, фасон, длину, вырез, рукава, ткань, цвет, обувь, аксессуары, причёску, макияж, место, свет, позу. Не добавляй возраст, цвет кожи, национальность, черты лица, фигуру и цвет волос. Ответ — только JSON {"texts":[...]} в том же порядке и того же количества, без пояснений.`

const translateRuSystemPrompt = `Ты — переводчик для стилиста. Переведи каждое описание одежды, причёски, макияжа или фона с английского на русский. Пиши только по-русски, грамотно, с правильными падежами и согласованием, как пишет живой стилист. Сохрани все детали: фасон, длину, вырез, рукава, ткань, цвет, обувь, аксессуары. Названия вещей — общепринятые русские (лоферы, клатч, брюки со стрелками). Ответ — только JSON {"texts":[...]} в том же порядке и того же количества, без пояснений.`

const translateRuTemperature = 0.3

var cjkRe = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]`)

var notEnglishRe = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}\p{Cyrillic}]`)

var errNoTranslation = errors.New("перевод не удался")

func Translate(chat Ideas, model string, texts []string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}
	if len(texts) > TranslateMax {
		return nil, fmt.Errorf("за раз не больше %d текстов, передано %d", TranslateMax, len(texts))
	}
	for _, v := range texts {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, errors.New("пустой текст для перевода")
		}
		if len([]rune(v)) > translateInMaxLen {
			return nil, fmt.Errorf("«%s»: текст длиннее %d символов", quoteShort(v), translateInMaxLen)
		}
	}
	if chat == nil || strings.TrimSpace(model) == "" {
		return nil, errors.New("модель ИИ не настроена")
	}
	in, err := json.Marshal(map[string][]string{"texts": texts})
	if err != nil {
		return nil, err
	}
	raw, err := chat.Chat(model, translateSystemPrompt, string(in), translateTemperature, translateMaxTokens)
	if err != nil {
		return nil, fmt.Errorf("ИИ не перевёл: %w", err)
	}
	got, err := parseTexts(raw, len(texts))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(texts))
	for i, v := range got {
		v = strings.TrimSpace(v)
		switch {
		case v == "" || notEnglishRe.MatchString(v):
			return nil, fmt.Errorf("«%s»: ИИ вернул пустой или неанглийский перевод", quoteShort(texts[i]))
		case len([]rune(v)) > ideaMaxLen:
			return nil, fmt.Errorf("«%s»: перевод длиннее %d символов", quoteShort(texts[i]), ideaMaxLen)
		case Forbidden(v):
			return nil, fmt.Errorf("«%s»: в переводе есть внешность человека (возраст, кожа, черты лица, фигура, цвет волос) — уберите её из текста", quoteShort(texts[i]))
		}
		out[i] = v
	}
	return out, nil
}

func parseTexts(raw string, want int) ([]string, error) {
	var parsed struct {
		Texts []string `json:"texts"`
	}
	if err := json.Unmarshal([]byte(promptutil.ExtractJSON(raw)), &parsed); err != nil {
		return nil, fmt.Errorf("ИИ ответил не JSON: %w", err)
	}
	if len(parsed.Texts) != want {
		return nil, fmt.Errorf("ИИ вернул переводов: %d из %d", len(parsed.Texts), want)
	}
	return parsed.Texts, nil
}

func TranslateToRu(chat Ideas, model string, texts []string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}
	if chat == nil || strings.TrimSpace(model) == "" {
		return nil, errors.New("модель ИИ не настроена")
	}
	in, err := json.Marshal(map[string][]string{"texts": texts})
	if err != nil {
		return nil, err
	}
	raw, err := chat.Chat(model, translateRuSystemPrompt, string(in), translateRuTemperature, translateMaxTokens)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoTranslation, err)
	}
	got, err := parseTexts(raw, len(texts))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoTranslation, err)
	}
	if cjkRe.MatchString(strings.Join(got, "\n")) {
		return nil, fmt.Errorf("%w: в ответе иероглифы", errNoTranslation)
	}
	out := make([]string, len(got))
	for i, v := range got {
		if v = strings.TrimSpace(v); ruAcceptable(v) {
			out[i] = v
		}
	}
	return out, nil
}

func AddRussian(chat Ideas, model string, ideas []Idea) error {
	var dst []*string
	var texts []string
	for i := range ideas {
		id := &ideas[i]
		for _, f := range [][2]*string{{&id.Outfit, &id.OutfitRu}, {&id.Hair, &id.HairRu}, {&id.Makeup, &id.MakeupRu}, {&id.Background, &id.BackgroundRu}} {
			if *f[0] != "" {
				texts = append(texts, *f[0])
				dst = append(dst, f[1])
			}
		}
	}
	if len(texts) == 0 {
		return nil
	}
	out, err := TranslateToRu(chat, model, texts)
	if err != nil {
		return err
	}
	for i, v := range out {
		*dst[i] = v
	}
	return nil
}

func quoteShort(v string) string {
	v = strings.TrimSpace(v)
	if r := []rune(v); len(r) > translateQuoteMaxRune {
		return string(r[:translateQuoteMaxRune-1]) + "…"
	}
	return v
}
