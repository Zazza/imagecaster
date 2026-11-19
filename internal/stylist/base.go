package stylist

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"sync"
)

//go:embed presets.json
var presetsJSON []byte

type Aspect string

const (
	AspectOutfit     Aspect = "outfit"
	AspectHair       Aspect = "hair"
	AspectMakeup     Aspect = "makeup"
	AspectBackground Aspect = "background"
	AspectPose       Aspect = "pose"
)

const PlaceAny = "any"

var framings = []string{"close", "waist", "knee", "full"}

type Theme struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Item struct {
	ID     string `json:"id"`
	Theme  string `json:"theme,omitempty"`
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type Background struct {
	Item
	Place string `json:"place"`
}

type Pose struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Prompt      string   `json:"prompt"`
	Framing     string   `json:"framing"`
	Places      []string `json:"places"`
	Themes      []string `json:"themes,omitempty"`
	Orientation string   `json:"orientation"`
}

type Base struct {
	Version    int          `json:"version"`
	Themes     []Theme      `json:"themes"`
	Outfit     []Item       `json:"outfit"`
	Hair       []Item       `json:"hair"`
	Makeup     []Item       `json:"makeup"`
	Background []Background `json:"background"`
	Pose       []Pose       `json:"pose"`
}

var (
	loadOnce sync.Once
	loaded   *Base
	loadErr  error
)

func Load() (*Base, error) {
	loadOnce.Do(func() {
		loaded, loadErr = Parse(presetsJSON)
	})
	return loaded, loadErr
}

func Parse(raw []byte) (*Base, error) {
	var b Base
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("база стилиста: %w", err)
	}
	if err := b.validate(); err != nil {
		return nil, fmt.Errorf("база стилиста: %w", err)
	}
	return &b, nil
}

func (b *Base) validate() error {
	themes := map[string]bool{}
	for _, t := range b.Themes {
		if t.ID == "" || t.Name == "" {
			return errors.New("тема без id или названия")
		}
		themes[t.ID] = true
	}
	seen := map[string]bool{}
	check := func(id, name, prompt, theme string) error {
		if id == "" || name == "" || prompt == "" {
			return fmt.Errorf("заготовка %q: пустое поле", id)
		}
		if seen[id] {
			return fmt.Errorf("заготовка %q повторяется", id)
		}
		seen[id] = true
		if theme != "" && !themes[theme] {
			return fmt.Errorf("заготовка %q: неизвестная тема %q", id, theme)
		}
		return nil
	}
	for _, list := range [][]Item{b.Outfit, b.Hair, b.Makeup} {
		if len(list) == 0 {
			return errors.New("пустой раздел")
		}
		for _, it := range list {
			if err := check(it.ID, it.Name, it.Prompt, it.Theme); err != nil {
				return err
			}
		}
	}
	if len(b.Background) == 0 || len(b.Pose) == 0 {
		return errors.New("нет фонов или поз")
	}
	places := map[string]bool{}
	for _, bg := range b.Background {
		if err := check(bg.ID, bg.Name, bg.Prompt, bg.Theme); err != nil {
			return err
		}
		if bg.Place == "" || bg.Place == PlaceAny {
			return fmt.Errorf("фон %q: нет места", bg.ID)
		}
		places[bg.Place] = true
	}
	anyPose := false
	for _, p := range b.Pose {
		if err := check(p.ID, p.Name, p.Prompt, ""); err != nil {
			return err
		}
		if !slices.Contains(framings, p.Framing) {
			return fmt.Errorf("поза %q: неизвестный план %q", p.ID, p.Framing)
		}
		if p.Orientation != "portrait" && p.Orientation != "landscape" {
			return fmt.Errorf("поза %q: неизвестная ориентация %q", p.ID, p.Orientation)
		}
		if len(p.Places) == 0 {
			return fmt.Errorf("поза %q: нет мест", p.ID)
		}
		for _, pl := range p.Places {
			if pl != PlaceAny && !places[pl] {
				return fmt.Errorf("поза %q: место %q не встречается у фонов", p.ID, pl)
			}
		}
		for _, th := range p.Themes {
			if !themes[th] {
				return fmt.Errorf("поза %q: неизвестная тема %q", p.ID, th)
			}
		}
		if slices.Contains(p.Places, PlaceAny) && len(p.Themes) == 0 {
			anyPose = true
		}
	}
	if !anyPose {
		return errors.New("нет ни одной позы для любого места без темы")
	}
	return nil
}

func (b *Base) Places() []string {
	set := map[string]bool{}
	for _, bg := range b.Background {
		set[bg.Place] = true
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (b *Base) Items(kind Aspect) []Item {
	switch kind {
	case AspectOutfit:
		return b.Outfit
	case AspectHair:
		return b.Hair
	case AspectMakeup:
		return b.Makeup
	case AspectBackground:
		out := make([]Item, 0, len(b.Background))
		for _, bg := range b.Background {
			out = append(out, bg.Item)
		}
		return out
	}
	return nil
}

func (b *Base) Find(kind Aspect, id string) (Item, bool) {
	for _, it := range b.Items(kind) {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

func (b *Base) PlaceOf(backgroundID string) string {
	for _, bg := range b.Background {
		if bg.ID == backgroundID {
			return bg.Place
		}
	}
	return ""
}

func (b *Base) FindPose(id string) (Pose, bool) {
	for _, p := range b.Pose {
		if p.ID == id {
			return p, true
		}
	}
	return Pose{}, false
}

func (b *Base) Pick(kind Aspect, themes []string, rnd *rand.Rand) Item {
	all := b.Items(kind)
	if len(all) == 0 {
		return Item{}
	}
	var match []Item
	for _, it := range all {
		if it.Theme == "" || len(themes) == 0 || slices.Contains(themes, it.Theme) {
			match = append(match, it)
		}
	}
	if len(match) == 0 {
		match = all
	}
	return match[rnd.Intn(len(match))]
}

func (b *Base) BackgroundTheme(id string) string {
	for _, bg := range b.Background {
		if bg.ID == id {
			return bg.Theme
		}
	}
	return ""
}

func (b *Base) PosesFor(place string, themes []string) []Pose {
	var out []Pose
	for _, p := range b.Pose {
		if len(p.Themes) > 0 && !slices.ContainsFunc(p.Themes, func(th string) bool { return slices.Contains(themes, th) }) {
			continue
		}
		if slices.Contains(p.Places, PlaceAny) || (place != "" && slices.Contains(p.Places, place)) {
			out = append(out, p)
		}
	}
	return out
}
