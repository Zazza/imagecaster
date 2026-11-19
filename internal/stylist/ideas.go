package stylist

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Zazza/imagecaster/internal/promptutil"
)

type Ideas interface {
	Chat(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error)
}

type Idea struct {
	Outfit       string `json:"outfit"`
	Hair         string `json:"hair"`
	Makeup       string `json:"makeup"`
	Background   string `json:"background"`
	Place        string `json:"place"`
	OutfitRu     string `json:"-"`
	HairRu       string `json:"-"`
	MakeupRu     string `json:"-"`
	BackgroundRu string `json:"-"`
}

const (
	ideaMaxLen      = 600
	outfitMinWords  = 6
	ideaTemperature = 0.9
	ideaMaxTokens   = 1500
	wishMaxLen      = 1000
)

var forbiddenRe = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\b\d+\s*-?\s*years?\s*-?\s*old\b`, `\byears old\b`, `\baged?\b`, `\byoung(er)?\b`, `\bteen(age|ager)?s?\b`, `\belderly\b`, `\bmiddle-aged\b`, `\bmature\b`,
	`\bskin[- ]?tones?\b`, `\bcomplexion\b`, `\b(pale|fair|dark|olive|tanned|light|brown|black|white)[- ]skin(ned)?\b`, `\btanned\b`, `\bsuntan\b`,
	`\b(asian|african|european|caucasian|latina|latino|hispanic|slavic|indian|arab|ethnic\w*|ethnicity|race)\b`,
	`\b(blue|green|brown|hazel|grey|gray|dark) eyes\b`, `\beye colou?r\b`, `\bnose\b`, `\bjaw(line)?\b`, `\bcheekbones?\b`, `\bfacial features\b`, `\bface shape\b`, `\b(full|thin|plump) lips\b`,
	`\b(curvy|petite|plus[- ]size|chubby|overweight|muscular|athletic build|busty|voluptuous|hourglass figure|body type|body shape)\b`, `\b(slim|slender|skinny|thin|tall|short)\s+(woman|man|girl|body|figure|build|waist|legs)\b`,
	`\b(blonde?s?|brunettes?|redhead(ed)?|ginger|auburn|balayage|ombr[eé]|highlights|lowlights|chestnut|jet[- ]black|raven|dyed|bleached|platinum)\b`, `\b\w+-haired\b`,
	`\bin (her|his|their) (early |mid-?|late )?\d0s\b`, `\b\d0s (woman|man|girl)\b`, `\byouthful\b`, `\bgirls?\b`, `\bboys?\b`,
	`\bporcelain\b`, `\b(tan|tanned|bronzed|sun-kissed|olive|fair|pale|dark|light)[- ]skin(ned)?\b`, `\bolive-toned\b`, `-toned skin\b`, `\btan lines?\b`,
	`\blong legs\b`, `\bslender\b`, `\bfull-figured\b`, `\bcurvaceous\b`, `\b(black|brown|red|grey|gray|silver|white|dark|light|golden|copper|pink|blue|purple|dyed|platinum)[- ]hair(ed)?\b`, `\bhair colou?r\b`,
}, "|"))

var allowedRe = regexp.MustCompile(`(?i)\bman[- ]buns?\b`)

func Forbidden(text string) bool {
	return forbiddenRe.MatchString(allowedRe.ReplaceAllString(text, ""))
}

const ruStart = `(?:^|[^\p{L}])`
const ruEnd = `(?:[^\p{L}]|$)`
const ruSkin = `кож(?:а|и|е|у|ей|ею)` + ruEnd

var forbiddenRuRe = regexp.MustCompile(`(?i)` + ruStart + `(?:` + strings.Join([]string{
	`возраст`, `\d+\s*-?\s*(?:лет|год)`, `лет` + ruEnd, `молод\p{L}*`, `юн(?:ая|ый|ой|ые|ость)` + ruEnd, `пожил\p{L}*`, `подрост\p{L}*`, `девочк\p{L}*`, `девушк\p{L}*`, `мальчик\p{L}*`,
	`(?:цвет\p{L}*|оттен\p{L}*|тон(?:ом|а|у)?)\s+` + ruSkin, `(?:светл|т[её]мн|бледн|смугл|оливков|фарфоров|загорел)\p{L}*\s+` + ruSkin, `смугл\p{L}*`, `загар\p{L}*`, `загорел\p{L}*`, `фарфоров\p{L}*\s+(?:кож|лиц)`,
	`национальн\p{L}*`, `этни\p{L}*`, `рас(?:а|ы|е|у|ой)` + ruEnd, `азиат\p{L}*`, `африкан\p{L}*`, `европе\p{L}*`, `славян\p{L}*`, `латин\p{L}*`, `арабск\p{L}*`, `индийск\p{L}*`, `кавказ\p{L}*`,
	`черт\p{L}*\s+лица`, `форм\p{L}*\s+лица`, `цвет\p{L}*\s+глаз`, `(?:голуб|зел[её]н|кар|сер|т[её]мн)\p{L}*\s+глаз\p{L}*`, `нос(?:а|ом|у)?` + ruEnd, `скул\p{L}*`, `подбород\p{L}*`, `челюст\p{L}*`, `(?:пухл|тонк|полн)\p{L}*\s+губ\p{L}*`,
	`фигур(?:а|ы|е|у|ой|ою)?` + ruEnd, `телосложен\p{L}*`, `худ(?:ая|ой|ые|ый|ощав\p{L}*|еньк\p{L}*)` + ruEnd, `полн(?:ая|ой|ые|ый|оват\p{L}*|отел\p{L}*)` + ruEnd, `стройн\p{L}*`, `пышногруд\p{L}*`, `мускулист\p{L}*`, `длинн\p{L}*\s+ног\p{L}*`, `(?:высок|низк)\p{L}*\s+(?:рост|женщин|девушк|мужчин)\p{L}*`,
	`блонд\p{L}*`, `брюнет\p{L}*`, `шатен\p{L}*`, `рыжеволос\p{L}*`, `рыж\p{L}*\s+(?:волос|локон|пряд|кудр|кос|шевелюр)\p{L}*`, `рус(?:ые|ый|ая|ых|ой|ою)` + ruEnd, `сед(?:ые|ой|ая|ых|ина|ину)` + ruEnd, `окраш\p{L}*`, `крашен\p{L}*`, `мелирован\p{L}*`, `балаяж\p{L}*`, `омбре`, `колорирован\p{L}*`, `цвет\p{L}*\s+волос\p{L}*`,
	`(?:ч[её]рн|светл|т[её]мн|золотист|медн|пепельн|платинов|каштанов|русы)\p{L}*\s+волос\p{L}*`,
}, "|") + `)`)

var ruAllowedRe = regexp.MustCompile(`по фигуре|до подбородка|в полный рост|европейск\p{L}*\s+стил\p{L}*|из\s+(?:\p{L}+\s+){0,2}кож(?:и|е)`)

func ForbiddenRu(text string) bool {
	return forbiddenRuRe.MatchString(ruAllowedRe.ReplaceAllString(strings.ToLower(text), ""))
}

func ideaSystemPrompt(fields []Aspect, places []string, count int) string {
	names := make([]string, 0, len(fields)+1)
	for _, f := range fields {
		names = append(names, string(f))
	}
	keys := make([]string, 0, len(names)+1)
	for _, n := range names {
		keys = append(keys, fmt.Sprintf(`"%s":"..."`, n))
	}
	if slices.Contains(fields, AspectBackground) {
		keys = append(keys, `"place":"..."`)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are a fashion stylist planning a photo shoot of one person. Reply with JSON only, no other text: {\"series\":[{%s}]}.\n", strings.Join(keys, ","))
	fmt.Fprintf(&b, "Give exactly %d series, all different from each other. Write in English, short comma-separated phrases for an image prompt.\n", count)
	for _, f := range fields {
		switch f {
		case AspectOutfit:
			b.WriteString("outfit: spell out EVERY garment separately: type, cut, length, neckline, sleeves, fabric, colour; then shoes and accessories. At least 10 words.\n")
		case AspectHair:
			b.WriteString("hair: the hairstyle only with some detail (length, texture, parting, how it is styled), never the hair colour.\n")
		case AspectMakeup:
			b.WriteString("makeup: the makeup only.\n")
		case AspectBackground:
			fmt.Fprintf(&b, "background: the location and the light.\nplace: exactly one of %s, matching the background.\n", strings.Join(places, ", "))
		}
	}
	b.WriteString("Never mention age, skin tone or complexion, ethnicity or race, facial features, body shape or size, hair colour.")
	return b.String()
}

func ideaUserMessage(req PlanRequest, count int) string {
	wish := strings.TrimSpace(req.Wish)
	if r := []rune(wish); len(r) > wishMaxLen {
		wish = string(r[:wishMaxLen])
	}
	if wish == "" {
		wish = "-"
	}
	themes := "-"
	if len(req.Themes) > 0 {
		themes = strings.Join(req.Themes, ", ")
	}
	msg := fmt.Sprintf("Wish: %s\nThemes: %s\nSeries: %d", wish, themes, count)
	if p := personOf(req.Gender); p != "" {
		msg = "Person: " + p + "\n" + msg
	}
	return msg
}

func personOf(gender string) string {
	switch strings.ToLower(strings.TrimSpace(gender)) {
	case "":
		return ""
	case "man", "male":
		return "a man"
	case "woman", "female":
		return "a woman"
	}
	return "a person"
}

func AskLLM(chat Ideas, model string, req PlanRequest, base *Base, count int) ([]Idea, []string, error) {
	fields := req.AutoAspects()
	if len(fields) == 0 || count < 1 {
		return nil, nil, nil
	}
	if chat == nil || strings.TrimSpace(model) == "" {
		return nil, nil, errors.New("модель ИИ не настроена")
	}
	places := base.Places()
	raw, err := chat.Chat(model, ideaSystemPrompt(fields, places, count), ideaUserMessage(req, count), ideaTemperature, ideaMaxTokens)
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		Series []Idea `json:"series"`
	}
	if err := json.Unmarshal([]byte(promptutil.ExtractJSON(raw)), &parsed); err != nil {
		return nil, nil, fmt.Errorf("ИИ ответил не JSON: %w", err)
	}
	if len(parsed.Series) == 0 {
		return nil, nil, errors.New("ИИ не предложил ни одной серии")
	}
	var warnings []string
	if len(parsed.Series) < count {
		warnings = append(warnings, fmt.Sprintf("ИИ предложил серий: %d из %d — остальные из заготовок", len(parsed.Series), count))
	}
	out := make([]Idea, count)
	for i := range min(count, len(parsed.Series)) {
		in := parsed.Series[i]
		var got Idea
		for _, f := range fields {
			var val, dst *string
			switch f {
			case AspectOutfit:
				val, dst = &in.Outfit, &got.Outfit
			case AspectHair:
				val, dst = &in.Hair, &got.Hair
			case AspectMakeup:
				val, dst = &in.Makeup, &got.Makeup
			case AspectBackground:
				val, dst = &in.Background, &got.Background
			}
			v := strings.TrimSpace(*val)
			reason := ideaProblem(v)
			if reason == "" && f == AspectOutfit && len(strings.Fields(v)) < outfitMinWords {
				reason = "слишком коротко"
			}
			if reason == "" && f == AspectBackground && !slices.Contains(places, strings.TrimSpace(in.Place)) {
				reason = "место не из списка"
			}
			if reason != "" {
				warnings = append(warnings, fmt.Sprintf("Серия %d, %s: идея ИИ не подошла (%s) — взята заготовка", i+1, strings.ToLower(aspectNames[f]), reason))
				continue
			}
			*dst = v
			if f == AspectBackground {
				got.Place = strings.TrimSpace(in.Place)
			}
		}
		out[i] = got
	}
	return out, warnings, nil
}

func ruAcceptable(v string) bool {
	return v != "" && len([]rune(v)) <= ideaMaxLen && !ForbiddenRu(v)
}

func ideaProblem(v string) string {
	switch {
	case v == "":
		return "пусто"
	case len([]rune(v)) > ideaMaxLen:
		return "слишком длинно"
	case Forbidden(v):
		return "внешность человека"
	}
	return ""
}
