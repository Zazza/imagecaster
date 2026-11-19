package stylist

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	UserNameMax   = 60
	UserPromptMax = 1000
	userIDPrefix  = "u"
)

type UserPreset struct {
	ID          int64    `json:"id"`
	Aspect      Aspect   `json:"aspect"`
	Name        string   `json:"name"`
	Prompt      string   `json:"prompt"`
	PromptRu    string   `json:"prompt_ru"`
	Place       string   `json:"place"`
	Framing     string   `json:"framing"`
	Places      []string `json:"places"`
	Orientation string   `json:"orientation"`
}

func UserID(id int64) string {
	return userIDPrefix + strconv.FormatInt(id, 10)
}

func ParseUserID(s string) (int64, bool) {
	digits, ok := strings.CutPrefix(s, userIDPrefix)
	if !ok || digits == "" || strings.TrimLeft(digits, "0123456789") != "" {
		return 0, false
	}
	id, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func CheckUserName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("нет названия заготовки")
	}
	if len([]rune(name)) > UserNameMax {
		return fmt.Errorf("название заготовки — не длиннее %d символов", UserNameMax)
	}
	return nil
}

func (b *Base) CheckUser(p UserPreset) error {
	switch p.Aspect {
	case AspectOutfit, AspectHair, AspectMakeup, AspectBackground, AspectPose:
	default:
		return fmt.Errorf("неизвестный раздел заготовки %q", p.Aspect)
	}
	if err := CheckUserName(p.Name); err != nil {
		return err
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return errors.New("пустой текст заготовки")
	}
	if len([]rune(p.Prompt)) > UserPromptMax || len([]rune(p.PromptRu)) > UserPromptMax {
		return fmt.Errorf("текст заготовки — не длиннее %d символов", UserPromptMax)
	}
	places := b.Places()
	switch p.Aspect {
	case AspectBackground:
		if !slices.Contains(places, p.Place) {
			return fmt.Errorf("фон: неизвестное место %q", p.Place)
		}
	case AspectPose:
		if !slices.Contains(framings, p.Framing) {
			return fmt.Errorf("поза: неизвестный план %q", p.Framing)
		}
		if len(p.Places) == 0 {
			return errors.New("поза: не выбраны места")
		}
		for _, pl := range p.Places {
			if pl != PlaceAny && !slices.Contains(places, pl) {
				return fmt.Errorf("поза: неизвестное место %q", pl)
			}
		}
		if p.Orientation != "portrait" && p.Orientation != "landscape" {
			return fmt.Errorf("поза: неизвестная ориентация %q", p.Orientation)
		}
	}
	return nil
}

func (b *Base) WithUser(list []UserPreset) (*Base, error) {
	out := *b
	out.Outfit = slices.Clone(b.Outfit)
	out.Hair = slices.Clone(b.Hair)
	out.Makeup = slices.Clone(b.Makeup)
	out.Background = slices.Clone(b.Background)
	out.Pose = slices.Clone(b.Pose)
	var errs []error
	for _, p := range list {
		if err := b.CheckUser(p); err != nil {
			errs = append(errs, fmt.Errorf("своя заготовка %s пропущена: %w", UserID(p.ID), err))
			continue
		}
		it := Item{ID: UserID(p.ID), Name: strings.TrimSpace(p.Name), Prompt: strings.TrimSpace(p.Prompt), PromptRu: strings.TrimSpace(p.PromptRu), User: true}
		switch p.Aspect {
		case AspectOutfit:
			out.Outfit = append(out.Outfit, it)
		case AspectHair:
			out.Hair = append(out.Hair, it)
		case AspectMakeup:
			out.Makeup = append(out.Makeup, it)
		case AspectBackground:
			out.Background = append(out.Background, Background{Item: it, Place: p.Place})
		case AspectPose:
			out.Pose = append(out.Pose, Pose{
				ID: it.ID, Name: it.Name, Prompt: it.Prompt, PromptRu: it.PromptRu, User: true,
				Framing: p.Framing, Places: slices.Clone(p.Places), Orientation: p.Orientation,
			})
		}
	}
	return &out, errors.Join(errs...)
}
