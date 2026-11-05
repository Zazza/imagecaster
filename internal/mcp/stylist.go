package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"golang.org/x/net/http/httpproxy"
)

type StylistEngine interface {
	KleinEdit(imageBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error)
	DescribeImage(imageBase64, prompt string) (string, error)
}

type AppSessions interface {
	AddToNamedSession(name, imageBase64 string, info json.RawMessage, source string, isPreview bool, meta *preset.GenerationMeta) int64
}

type AgentSessions struct {
	Svc AppSessions
}

func (a AgentSessions) AddToSession(imageBase64 string, info json.RawMessage, source string, isPreview bool, meta *preset.GenerationMeta) int64 {
	return a.Svc.AddToNamedSession(AgentSessionName, imageBase64, info, source, isPreview, meta)
}

type Look struct {
	HairCut   string `json:"hair_cut,omitempty"`
	HairColor string `json:"hair_color,omitempty"`
	Outfit    string `json:"outfit,omitempty"`
	Glasses   string `json:"glasses,omitempty"`
	Makeup    string `json:"makeup,omitempty"`
	Free      string `json:"free,omitempty"`
}

const (
	AgentSessionName    = "ИИ-агент"
	StylistSessionName  = "Стилист"
	stylistMaxVariants  = 4
	stylistDefaultCount = 2
	stylistLabelMax     = 80
	stylistKeepSuffix   = "Keep the person's face, identity, pose and background unchanged."
	stylistAskUser      = "Спроси у человека форму лица, возраст и тон кожи — по фото они определяются ненадёжно."
	stylistNote         = "Варианты — в приложении, сессия «Стилист»; человек открывает их там. Файлы не открывай: фото не покидают локальную сеть, результат описан в check."
	stylistDescribe     = "Describe the person's current styling for a stylist, in short lines:\nHair: length (where it ends), cut, color, texture, parting.\nEyewear: none, or frame shape and color.\nClothing: each visible garment with type, color, material.\nAccessories: jewelry, hats, bags, or none.\nMakeup: lips and eyes, or none.\nBackground and lighting: one line.\nDescribe only what is visible. Do not guess age, face shape or skin tone."
	stylistCheck        = "Look at the person in this photo and describe their current styling in four short lines:\nHair: length (where it ends), cut name, color.\nEyewear: none, or frame shape/color and whether lenses are clear or tinted.\nOuter clothing: garment type, material, color.\nMakeup: lips and eyes.\nDescribe only what is visible."
)

var lookSchema = prop("образ (по-английски): hair_cut, hair_color, outfit, glasses (\"none\" — снять; для очков пиши \"clear lenses\", иначе стёкла тонируются), makeup, free — любые поля", "object",
	map[string]any{"properties": map[string]any{
		"hair_cut": prop("стрижка", "string"), "hair_color": prop("цвет волос", "string"),
		"outfit": prop("одежда", "string"), "glasses": prop("очки", "string"),
		"makeup": prop("макияж", "string"), "free": prop("прочие изменения", "string"),
	}})

func buildLookInstruction(l Look) string {
	var parts []string
	add := func(format, v string) {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, fmt.Sprintf(format, strings.TrimRight(v, ". ")))
		}
	}
	add("Change the hairstyle to %s.", l.HairCut)
	add("Recolor the hair %s.", l.HairColor)
	add("Replace the current clothing with %s.", l.Outfit)
	switch strings.ToLower(strings.TrimSpace(l.Glasses)) {
	case "":
	case "none", "no", "remove", "нет":
		parts = append(parts, "Remove the eyeglasses.")
	default:
		add("Add %s.", l.Glasses)
	}
	add("Apply makeup: %s.", l.Makeup)
	add("%s.", l.Free)
	parts = append(parts, stylistKeepSuffix)
	return strings.Join(parts, " ")
}

func lookLabel(l Look) string {
	var parts []string
	for _, v := range []string{l.HairCut, l.HairColor, l.Outfit, l.Glasses, l.Makeup, l.Free} {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	label := strings.Join(parts, ", ")
	if r := []rune(label); len(r) > stylistLabelMax {
		label = string(r[:stylistLabelMax]) + "…"
	}
	return label
}

func (s *Server) stylistDeps() (StylistEngine, AppSessions, error) {
	if s.deps.Stylist == nil || s.deps.Sessions == nil {
		return nil, nil, errors.New("стилист недоступен в этой сборке sd-mcp")
	}
	if cfg := s.deps.Cfg; cfg != nil {
		for name, u := range map[string]string{"LLM": cfg.LLMUrl, "SD": cfg.SDUrl} {
			if !isLocalURL(u) {
				return nil, nil, fmt.Errorf("стилист отключён: адрес %s не в локальной сети, фото не могут покидать LAN", name)
			}
			if viaProxy(u) {
				return nil, nil, fmt.Errorf("стилист отключён: запросы к %s пошли бы через HTTP-прокси из окружения — добавь адрес в NO_PROXY", name)
			}
		}
	}
	return s.deps.Stylist, s.deps.Sessions, nil
}

func isLocalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	for _, ip := range ips {
		if !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
			return false
		}
	}
	return true
}

func viaProxy(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	p, err := httpproxy.FromEnvironment().ProxyFunc()(u)
	return err != nil || p != nil
}

func readPhotoArg(args map[string]any) (string, error) {
	path := argString(args, "photo_path")
	if path == "" {
		return "", errors.New("photo_path обязателен")
	}
	img, err := filebrowser.ReadFileAsBase64(path)
	if err != nil {
		return "", errors.New("не удалось прочитать фото по photo_path — проверь путь и формат")
	}
	return img, nil
}

func variantCount(args map[string]any) (int, error) {
	n := optInt(args, "variants")
	if n == nil {
		return stylistDefaultCount, nil
	}
	if *n < 1 || *n > stylistMaxVariants {
		return 0, fmt.Errorf("variants должно быть от 1 до %d", stylistMaxVariants)
	}
	return int(*n), nil
}

func stylistTry(eng StylistEngine, sessions AppSessions, photo string, look Look, count int) (string, error) {
	instruction := buildLookInstruction(look)
	meta := &preset.GenerationMeta{Label: "Стилист: " + lookLabel(look)}
	used := map[int64]bool{}
	var made []map[string]any
	var genErr error
	for range count {
		seed := rand.Int64N(1 << 31)
		for used[seed] {
			seed = rand.Int64N(1 << 31)
		}
		used[seed] = true
		res, err := eng.KleinEdit(photo, instruction, &seed)
		if err != nil {
			genErr = err
			break
		}
		id := sessions.AddToNamedSession(StylistSessionName, res.Image, res.Info, "stylist", false, meta)
		if id == 0 {
			genErr = errors.New("не удалось сохранить вариант в сессию приложения")
			break
		}
		check, err := eng.DescribeImage(res.Image, stylistCheck)
		if err != nil {
			check = "не удалось описать результат: " + err.Error()
		}
		made = append(made, map[string]any{"id": id, "check": check})
	}
	if len(made) == 0 {
		return "", fmt.Errorf("ни один вариант не получился: %w", genErr)
	}
	out := map[string]any{
		"session":     StylistSessionName,
		"instruction": instruction,
		"variants":    made,
		"note":        stylistNote,
	}
	if genErr != nil {
		out["failed"] = count - len(made)
		out["error"] = genErr.Error()
	}
	return toJSON(out), nil
}

func registerStylistTools(s *Server) {
	s.Register(Tool{
		Name: "stylist_describe",
		Description: "Стилист: описать внешний вид человека на фото ЛОКАЛЬНОЙ vision-моделью — в ответе только текст. " +
			"Сам фото не открывай и не читай — фото не должны покидать локальную сеть. Форму лица, возраст и тон кожи спроси у человека.",
		InputSchema: props(map[string]any{
			"photo_path": prop("путь к фото человека", "string"),
		}, "photo_path"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			eng, _, err := s.stylistDeps()
			if err != nil {
				return "", err
			}
			img, err := readPhotoArg(args)
			if err != nil {
				return "", err
			}
			description, err := eng.DescribeImage(img, stylistDescribe)
			if err != nil {
				return "", fmt.Errorf("описание фото: %w", err)
			}
			return toJSON(map[string]any{"description": description, "ask_user": stylistAskUser}), nil
		},
	})

	s.Register(Tool{
		Name: "stylist_try",
		Description: "Стилист: примерить образ на фото. Каждый вариант — одна правка ИСХОДНОГО фото (flux-2-klein), 8–25 с на вариант; варианты сразу появляются в приложении в сессии «Стилист». " +
			"Варианты различаются сидом (слабо) — разные идеи давай разными вызовами. Уточнение — снова stylist_try с тем же photo_path и полным изменённым look (правка поверх правки не делается). " +
			"Ответ: id элементов сессии и check — короткое описание результата локальной vision-моделью (тонировку стёкол не различает). Картинок и путей в ответе нет.",
		InputSchema: props(map[string]any{
			"photo_path": prop("путь к исходному фото человека", "string"),
			"look":       lookSchema,
			"variants":   prop("сколько вариантов 1–4 (по умолчанию 2)", "integer"),
		}, "photo_path", "look"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			eng, sessions, err := s.stylistDeps()
			if err != nil {
				return "", err
			}
			look, err := decodeArg[Look](args, "look")
			if err != nil {
				return "", err
			}
			if look == nil || *look == (Look{}) {
				return "", errors.New("look пуст — укажи хотя бы одно изменение")
			}
			count, err := variantCount(args)
			if err != nil {
				return "", err
			}
			img, err := readPhotoArg(args)
			if err != nil {
				return "", err
			}
			return stylistTry(eng, sessions, img, *look, count)
		},
	})
}
