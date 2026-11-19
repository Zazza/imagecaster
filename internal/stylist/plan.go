package stylist

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"slices"
	"strings"
)

type Mode string

const (
	ModeAuto Mode = "auto"
	ModeKeep Mode = "keep"
	ModePick Mode = "pick"
)

const (
	MaxFrames       = 24
	FramesPerSeries = 4
)

const (
	warnNoIdeas = "ИИ не дал идей — взяты случайные заготовки по темам"
	warnNoPick  = "%s: выбранные заготовки не найдены — взята случайная"
)

var aspectNames = map[Aspect]string{
	AspectOutfit:     "Одежда",
	AspectHair:       "Причёска",
	AspectMakeup:     "Макияж",
	AspectBackground: "Фон",
	AspectPose:       "Позы",
}

type AspectChoice struct {
	Mode Mode     `json:"mode"`
	IDs  []string `json:"ids"`
}

type PlanRequest struct {
	Person  string                  `json:"person"`
	Gender  string                  `json:"gender"`
	Wish    string                  `json:"wish"`
	Frames  int                     `json:"frames"`
	Themes  []string                `json:"themes"`
	Aspects map[string]AspectChoice `json:"aspects"`
	Garment bool                    `json:"garment"`
	Seed    int64                   `json:"seed"`
	Locale  string                  `json:"locale,omitempty"`
}

type Shot struct {
	Pose      string `json:"pose"`
	Prompt    string `json:"prompt"`
	PromptRu  string `json:"prompt_ru,omitempty"`
	Landscape bool   `json:"landscape"`
	Framing   string `json:"framing,omitempty"`
}

type Series struct {
	Outfit       string   `json:"outfit"`
	Hair         string   `json:"hair"`
	Makeup       string   `json:"makeup"`
	Background   string   `json:"background"`
	Place        string   `json:"place"`
	OutfitRu     string   `json:"outfit_ru,omitempty"`
	HairRu       string   `json:"hair_ru,omitempty"`
	MakeupRu     string   `json:"makeup_ru,omitempty"`
	BackgroundRu string   `json:"background_ru,omitempty"`
	RuAI         []string `json:"ru_ai,omitempty"`
	Shots        []Shot   `json:"shots"`
}

func (r PlanRequest) Choice(a Aspect) AspectChoice {
	if a == AspectOutfit && r.Garment {
		return AspectChoice{Mode: ModeKeep}
	}
	c, ok := r.Aspects[string(a)]
	if !ok || (c.Mode != ModeKeep && c.Mode != ModePick) {
		return AspectChoice{Mode: ModeAuto, IDs: c.IDs}
	}
	return c
}

func (r PlanRequest) AutoAspects() []Aspect {
	var out []Aspect
	for _, a := range []Aspect{AspectOutfit, AspectHair, AspectMakeup, AspectBackground} {
		if r.Choice(a).Mode == ModeAuto {
			out = append(out, a)
		}
	}
	return out
}

func SeriesSizes(frames int) []int {
	var out []int
	for frames > 0 {
		n := min(frames, FramesPerSeries)
		out = append(out, n)
		frames -= n
	}
	return out
}

func CheckFrames(frames int) error {
	if frames < 1 || frames > MaxFrames {
		return fmt.Errorf("кадров от 1 до %d, указано %d", MaxFrames, frames)
	}
	return nil
}

func Plan(req PlanRequest, base *Base, ideas []Idea) ([]Series, []string, error) {
	if base == nil {
		return nil, nil, errors.New("нет базы заготовок")
	}
	if err := CheckFrames(req.Frames); err != nil {
		return nil, nil, err
	}
	rnd := rand.New(rand.NewSource(req.Seed))
	var warnings []string
	if ideas == nil && len(req.AutoAspects()) > 0 {
		warnings = append(warnings, warnNoIdeas)
	}
	sizes := SeriesSizes(req.Frames)
	out := make([]Series, 0, len(sizes))
	frame := 0
	for i, n := range sizes {
		s, w := buildSeries(req, base, ideaAt(ideas, i), i, n, frame, rnd)
		out = append(out, s)
		warnings = appendUnique(warnings, w...)
		frame += n
	}
	return out, warnings, nil
}

func RegenerateSeries(req PlanRequest, base *Base, idea *Idea, index int) (Series, []string, error) {
	if base == nil {
		return Series{}, nil, errors.New("нет базы заготовок")
	}
	if err := CheckFrames(req.Frames); err != nil {
		return Series{}, nil, err
	}
	sizes := SeriesSizes(req.Frames)
	if index < 0 || index >= len(sizes) {
		return Series{}, nil, fmt.Errorf("серии %d нет", index+1)
	}
	frame := 0
	for _, n := range sizes[:index] {
		frame += n
	}
	var warnings []string
	if idea == nil && len(req.AutoAspects()) > 0 {
		warnings = append(warnings, warnNoIdeas)
	}
	rnd := rand.New(rand.NewSource(req.Seed))
	s, w := buildSeries(req, base, idea, index, sizes[index], frame, rnd)
	return s, appendUnique(warnings, w...), nil
}

const warnCloseGarment = "С фото вещи крупные и поясные позы получаются плохо: примерка втискивает вещь целиком в кадр и ломает пропорции (огромная голова). Лучше по колено и в рост."

var framingLead = map[string]string{
	"close": "close-up portrait, head and shoulders",
	"waist": "waist-up shot, upper body",
	"knee":  "knee-up shot",
	"full":  "full body shot",
}

var (
	footwear   = regexp.MustCompile(`(?i)\b(shoes?|heels?|stilettos?|pumps|sandals?|sneakers?|trainers|boots?|loafers|flats|espadrilles|mules|slippers|socks|stockings|tights)\b`)
	lengthWord = regexp.MustCompile(`(?i)\s*\b(floor[- ]length|ankle[- ]length|knee[- ]length|mid[- ]calf|calf[- ]length|maxi|midi|(reaching |down )?to the (floor|knees?|ankles?))\b`)
)

func upperOutfit(outfit string) string {
	var kept []string
	for _, p := range strings.Split(outfit, ",") {
		if footwear.MatchString(p) {
			continue
		}
		if p = strings.TrimSpace(lengthWord.ReplaceAllString(p, "")); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}

func FramePrompt(lora, appearance string, s Series, sh Shot) string {
	outfit := s.Outfit
	if sh.Framing == "close" || sh.Framing == "waist" {
		outfit = upperOutfit(outfit)
	}
	return ComposePrompt(lora, []string{framingLead[sh.Framing], sh.Prompt, appearance, outfit, s.Hair, s.Makeup}, s.Background)
}

func ideaAt(ideas []Idea, i int) *Idea {
	if i < len(ideas) {
		return &ideas[i]
	}
	return nil
}

func buildSeries(req PlanRequest, base *Base, idea *Idea, index, shots, frame int, rnd *rand.Rand) (Series, []string) {
	var warnings, ruAI []string
	field := func(a Aspect, fromIdea, fromIdeaRu string) Item {
		c := req.Choice(a)
		switch c.Mode {
		case ModeKeep:
			return Item{}
		case ModePick:
			if it, ok := pickRound(base, a, c.IDs, index); ok {
				return it
			}
			warnings = append(warnings, fmt.Sprintf(warnNoPick, aspectNames[a]))
		default:
			if fromIdea != "" {
				if fromIdeaRu != "" {
					ruAI = append(ruAI, string(a))
				}
				return Item{Prompt: fromIdea, Name: fromIdeaRu}
			}
		}
		return base.Pick(a, req.Themes, rnd)
	}
	var id Idea
	if idea != nil {
		id = *idea
	}
	outfit := field(AspectOutfit, id.Outfit, id.OutfitRu)
	hair := field(AspectHair, id.Hair, id.HairRu)
	makeup := field(AspectMakeup, id.Makeup, id.MakeupRu)
	s := Series{
		Outfit: outfit.Prompt, OutfitRu: outfit.Name,
		Hair: hair.Prompt, HairRu: hair.Name,
		Makeup: makeup.Prompt, MakeupRu: makeup.Name,
	}
	if req.Garment {
		s.Outfit, s.OutfitRu = DefaultOutfit, ""
		ruAI = slices.DeleteFunc(ruAI, func(f string) bool { return f == string(AspectOutfit) })
	}
	bgIdea, bgIdeaRu := "", ""
	if id.Place != "" {
		bgIdea, bgIdeaRu = id.Background, id.BackgroundRu
	}
	bg := field(AspectBackground, bgIdea, bgIdeaRu)
	s.Background, s.BackgroundRu = bg.Prompt, bg.Name
	s.RuAI = ruAI
	if bg.ID != "" {
		s.Place = base.PlaceOf(bg.ID)
	} else if bg.Prompt != "" {
		s.Place = id.Place
	}
	themes := req.Themes
	if th := base.BackgroundTheme(bg.ID); th != "" {
		themes = append(slices.Clone(themes), th)
	}
	s.Shots, warnings = buildShots(req, base, s.Place, themes, shots, frame, rnd, warnings)
	return s, warnings
}

func pickRound(base *Base, a Aspect, ids []string, index int) (Item, bool) {
	var valid []Item
	for _, id := range ids {
		if it, ok := base.Find(a, id); ok {
			valid = append(valid, it)
		}
	}
	if len(valid) == 0 {
		return Item{}, false
	}
	return valid[index%len(valid)], true
}

func shotOf(p Pose) Shot {
	return Shot{Pose: p.Name, Prompt: p.Prompt, PromptRu: p.Name, Landscape: p.Orientation == "landscape", Framing: p.Framing}
}

func buildShots(req PlanRequest, base *Base, place string, themes []string, n, frame int, rnd *rand.Rand, warnings []string) ([]Shot, []string) {
	out := make([]Shot, 0, n)
	c := req.Choice(AspectPose)
	switch c.Mode {
	case ModeKeep:
		for range n {
			out = append(out, Shot{})
		}
		return out, warnings
	case ModePick:
		var valid []Pose
		for _, id := range c.IDs {
			if p, ok := base.FindPose(id); ok {
				valid = append(valid, p)
			}
		}
		if len(valid) > 0 {
			if req.Garment && slices.ContainsFunc(valid, func(p Pose) bool { return garmentTight(p.Framing) }) {
				warnings = append(warnings, warnCloseGarment)
			}
			for j := range n {
				out = append(out, shotOf(valid[(frame+j)%len(valid)]))
			}
			return out, warnings
		}
		warnings = append(warnings, fmt.Sprintf(warnNoPick, aspectNames[AspectPose]))
	}
	return autoShots(base, place, themes, n, rnd, req.Garment), warnings
}

func garmentTight(framing string) bool {
	return framing == "close" || framing == "waist"
}

func autoShots(base *Base, place string, themes []string, n int, rnd *rand.Rand, garment bool) []Shot {
	pool := base.PosesFor(place, themes)
	cycle := framings
	if garment {
		pool = slices.DeleteFunc(slices.Clone(pool), func(p Pose) bool { return garmentTight(p.Framing) })
		cycle = slices.DeleteFunc(slices.Clone(framings), garmentTight)
	}
	used := map[string]bool{}
	out := make([]Shot, 0, n)
	for j := range n {
		f := cycle[j%len(cycle)]
		var cands []Pose
		for _, p := range pool {
			if p.Framing == f && !used[p.ID] {
				cands = append(cands, p)
			}
		}
		if len(cands) == 0 {
			for _, p := range pool {
				if !used[p.ID] {
					cands = append(cands, p)
				}
			}
		}
		if len(cands) == 0 {
			cands = pool
		}
		if len(cands) == 0 {
			out = append(out, Shot{})
			continue
		}
		p := cands[rnd.Intn(len(cands))]
		used[p.ID] = true
		out = append(out, shotOf(p))
	}
	return out
}

func appendUnique(list []string, add ...string) []string {
	for _, v := range add {
		dup := false
		for _, e := range list {
			if e == v {
				dup = true
				break
			}
		}
		if !dup {
			list = append(list, v)
		}
	}
	return list
}
