package mcp

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/finish"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
)

type EditorEngine interface {
	DescribeImage(imageBase64, prompt string) (string, error)
	FaceLandmarks(imageBase64 string) ([]faceguard.Point, error)
	KleinEffect(imageBase64, prompt string, strength float64) (*generation.GenerateImageResult, error)
	EffectPresets() ([]preset.Preset, error)
}

type EditorAdapter struct {
	Gen *generation.Service
	DB  *preset.DB
}

func (a EditorAdapter) DescribeImage(img, prompt string) (string, error) {
	return a.Gen.DescribeImage(img, prompt)
}

func (a EditorAdapter) FaceLandmarks(img string) ([]faceguard.Point, error) {
	return a.Gen.FaceLandmarks(img)
}

func (a EditorAdapter) KleinEffect(img, prompt string, strength float64) (*generation.GenerateImageResult, error) {
	return a.Gen.KleinEffect(img, prompt, strength)
}

func (a EditorAdapter) EffectPresets() ([]preset.Preset, error) {
	return a.DB.List()
}

const (
	EditorSessionName     = "Фоторедактор"
	editorSource          = "editor"
	editorDefaultStrength = 0.5
	editorDescribe        = "Describe this photo for a photo editor, in short lines:\nScene: what and where.\nPeople: how many, framing (close-up, half body, full body).\nLight: direction, hardness, time of day, light sources.\nColors: dominant colors and overall tint.\nMood: one line.\nDescribe only what is visible."
	editorNote            = "Результат — в приложении, сессия «Фоторедактор» (или та же сессия, что item); человек открывает его там. Файлы не открывай: фото не покидают локальную сеть."
	editorEffectWarning   = "эффект перерисовал кадр нейросетью: лицо и мелкие детали могут отличаться от исходника — скажи человеку сравнить"
)

var editorEffectGroups = map[string]string{
	"post":            "base",
	"post_light":      "light",
	"post_film":       "film",
	"post_weather":    "weather",
	"post_background": "background",
}

type editorFinish struct {
	finish.Params
	Focus string `json:"focus,omitempty"`
}

type editorEffect struct {
	Name     string   `json:"name"`
	Strength *float64 `json:"strength,omitempty"`
}

func (s *Server) editorDeps() (EditorEngine, AppSessions, error) {
	if s.deps.Editor == nil || s.deps.Sessions == nil {
		return nil, nil, errors.New("фоторедактор недоступен в этой сборке imagecaster-mcp")
	}
	if err := s.localOnly("фоторедактор"); err != nil {
		return nil, nil, err
	}
	return s.deps.Editor, s.deps.Sessions, nil
}

func decodeImage(b64 string) (image.Image, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.New("фото не декодируется")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("фото не декодируется")
	}
	return img, nil
}

func encodePNG(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func photoMeasures(img image.Image) map[string]any {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	step := max(1, int(math.Sqrt(float64(w*h)/250000)))
	var n, sumL, sumL2, sumS float64
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, _ := img.At(x, y).RGBA()
			rf, gf, bf := float64(r>>8), float64(g>>8), float64(bl>>8)
			l := 0.299*rf + 0.587*gf + 0.114*bf
			mx := math.Max(rf, math.Max(gf, bf))
			mn := math.Min(rf, math.Min(gf, bf))
			sumS += (mx - mn) / 255
			sumL += l
			sumL2 += l * l
			n++
		}
	}
	mean := sumL / n
	contrast := math.Sqrt(math.Max(0, sumL2/n-mean*mean))
	sat := sumS / n
	auto := adjust.AnalyzeAuto(img)
	orient := "квадрат"
	if w > h {
		orient = "горизонтальное"
	} else if h > w {
		orient = "вертикальное"
	}
	exposure := "нормальная"
	switch {
	case auto.Gamma > 1.15:
		exposure = "темновато"
	case auto.Gamma < 0.87:
		exposure = "светло"
	}
	cast := "нейтральный"
	switch {
	case auto.Temperature <= -15:
		cast = "тёплый (жёлтый/оранжевый)"
	case auto.Temperature >= 15:
		cast = "холодный (синий)"
	}
	return map[string]any{
		"width": w, "height": h, "orientation": orient,
		"exposure":        exposure,
		"mean_brightness": math.Round(mean),
		"contrast":        math.Round(contrast),
		"saturation":      round3(sat),
		"color_cast":      cast,
		"auto_suggest": map[string]any{
			"black_point": math.Round(auto.BlackPoint), "white_point": math.Round(auto.WhitePoint),
			"gamma": round3(auto.Gamma), "temperature": math.Round(auto.Temperature),
		},
	}
}

func faceBox(pts []faceguard.Point, w, h int) (cx, cy, share float64, ok bool) {
	if len(pts) == 0 || w <= 0 || h <= 0 {
		return 0, 0, 0, false
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
		minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
	}
	cx = (minX + maxX) / 2 / float64(w)
	cy = (minY + maxY) / 2 / float64(h)
	share = (maxY - minY) / float64(h)
	return cx, cy, share, true
}

func faceInfo(eng EditorEngine, b64 string, img image.Image) (map[string]any, float64, float64, bool) {
	pts, err := eng.FaceLandmarks(b64)
	if err != nil {
		return map[string]any{"found": false, "error": "поиск лица не удался (сервер недоступен или ошибся) — это не значит, что лица нет"}, 0, 0, false
	}
	if len(pts) == 0 {
		return map[string]any{"found": false}, 0, 0, false
	}
	cx, cy, share, ok := faceBox(pts, img.Bounds().Dx(), img.Bounds().Dy())
	if !ok {
		return map[string]any{"found": false}, 0, 0, false
	}
	return map[string]any{"found": true, "center_x": round3(cx), "center_y": round3(cy), "height_share": round3(share)}, cx, cy, true
}

func effectList(eng EditorEngine) (map[string][]string, map[string]preset.Preset, error) {
	all, err := eng.EffectPresets()
	if err != nil {
		return nil, nil, fmt.Errorf("список эффектов: %w", err)
	}
	groups := map[string][]string{}
	byName := map[string]preset.Preset{}
	for _, p := range all {
		g, ok := editorEffectGroups[p.PresetType]
		if !ok {
			continue
		}
		groups[g] = append(groups[g], p.Name)
		byName[strings.ToLower(p.Name)] = p
	}
	for g := range groups {
		sort.Strings(groups[g])
	}
	return groups, byName, nil
}

func parseEditorAdjust(raw any) (adjust.Params, bool, error) {
	var p adjust.Params
	if raw == nil {
		return p, false, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return p, false, errors.New("adjust — объект с числами")
	}
	fields := map[string]*float64{
		"brightness": &p.Brightness, "contrast": &p.Contrast, "black_point": &p.BlackPoint,
		"white_point": &p.WhitePoint, "gamma": &p.Gamma, "saturation": &p.Saturation,
		"temperature": &p.Temperature, "balance_r": &p.BalanceR, "balance_g": &p.BalanceG,
		"balance_b": &p.BalanceB, "sharpen": &p.Sharpen, "vignette": &p.Vignette,
	}
	for k, v := range m {
		dst, known := fields[k]
		if !known {
			return p, false, fmt.Errorf("adjust: неизвестная поправка %q", k)
		}
		f, isNum := v.(float64)
		if !isNum || math.IsNaN(f) || math.IsInf(f, 0) {
			return p, false, fmt.Errorf("adjust.%s — число", k)
		}
		r := adjust.Ranges[k]
		if f < r[0] || f > r[1] {
			return p, false, fmt.Errorf("adjust.%s: допустимо %s…%s", k, strconv.FormatFloat(r[0], 'f', -1, 64), strconv.FormatFloat(r[1], 'f', -1, 64))
		}
		*dst = f
	}
	return p, !p.IsNeutral(), nil
}

func focusOffset(img image.Image, ratio string, cx, cy float64) float64 {
	w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	a, b, _ := strings.Cut(ratio, ":")
	x, _ := strconv.ParseFloat(a, 64)
	y, _ := strconv.ParseFloat(b, 64)
	r := x / y
	if w/h > r {
		nw := h * r
		if w-nw <= 0 {
			return 0.5
		}
		return math.Min(1, math.Max(0, (cx*w-nw/2)/(w-nw)))
	}
	nh := w / r
	if h-nh <= 0 {
		return 0.5
	}
	return math.Min(1, math.Max(0, (cy*h-nh*0.4)/(h-nh)))
}

func editorSave(sessions AppSessions, src photoSource, img string, label string) int64 {
	meta := &preset.GenerationMeta{Label: "Фоторедактор: " + label}
	if src.item != nil {
		meta.SourceItem = src.item.ID
		if items, ok := sessions.(AppSessionItems); ok {
			return items.AddToSessionID(src.item.SessionID, img, nil, editorSource, false, meta)
		}
		return 0
	}
	return sessions.AddToNamedSession(EditorSessionName, img, nil, editorSource, false, meta)
}

func editorApply(eng EditorEngine, sessions AppSessions, src photoSource, args map[string]any) (string, error) {
	adj, hasAdj, err := parseEditorAdjust(args["adjust"])
	if err != nil {
		return "", err
	}
	eff, err := decodeArg[editorEffect](args, "effect")
	if err != nil {
		return "", err
	}
	fin, err := decodeArg[editorFinish](args, "finish")
	if err != nil {
		return "", err
	}
	if fin == nil {
		fin = &editorFinish{}
	}
	if err := fin.Params.Validate(); err != nil {
		return "", err
	}
	switch fin.Focus {
	case "", "center", "face":
	default:
		return "", errors.New("finish.focus: \"face\" или \"center\"")
	}
	if fin.Focus != "" && fin.Offset != nil {
		return "", errors.New("finish: укажи что-то одно — focus или offset")
	}
	hasEffect := eff != nil && strings.TrimSpace(eff.Name) != ""
	if !hasAdj && !hasEffect && fin.Params.IsNeutral() {
		return "", errors.New("нечего применять: укажи adjust, effect или finish")
	}

	var applied []string
	notes := map[string]any{}
	cur := src.img
	if hasEffect {
		_, byName, err := effectList(eng)
		if err != nil {
			return "", err
		}
		p, ok := byName[strings.ToLower(strings.TrimSpace(eff.Name))]
		if !ok {
			return "", fmt.Errorf("эффект %q не найден — возьми название из effects в ответе editor_describe", eff.Name)
		}
		strength := editorDefaultStrength
		if p.DenoisingStrength != nil && *p.DenoisingStrength > 0 {
			strength = *p.DenoisingStrength
		}
		if eff.Strength != nil {
			if *eff.Strength < 0.2 || *eff.Strength > 0.9 {
				return "", errors.New("effect.strength: от 0.2 до 0.9")
			}
			strength = *eff.Strength
		}
		res, err := eng.KleinEffect(cur, p.Prompt, strength)
		if err != nil {
			return "", fmt.Errorf("эффект: %w", err)
		}
		cur = res.Image
		applied = append(applied, fmt.Sprintf("эффект %s %.2f", p.Name, strength))
		notes["warning"] = editorEffectWarning
	}

	img, err := decodeImage(cur)
	if err != nil {
		return "", err
	}
	if hasAdj {
		img = adjust.Apply(img, adj)
		applied = append(applied, "поправки")
	}
	if !fin.Params.IsNeutral() {
		fp := fin.Params
		if fp.Ratio != "" && (fp.Fit == "" || fp.Fit == "crop") && fin.Focus == "face" {
			b64, err := encodePNG(img)
			if err != nil {
				return "", err
			}
			if face, cx, cy, ok := faceInfo(eng, b64, img); ok {
				off := focusOffset(img, fp.Ratio, cx, cy)
				fp.Offset = &off
			} else if face["error"] != nil {
				notes["focus"] = "поиск лица не удался — кадр обрезан по центру"
			} else {
				notes["focus"] = "лицо не найдено — кадр обрезан по центру"
			}
		}
		out, err := finish.Apply(img, fp)
		if err != nil {
			return "", err
		}
		img = out
		if fp.Ratio != "" {
			applied = append(applied, "формат "+fp.Ratio)
		}
		if fp.Frame != "" {
			applied = append(applied, "рамка "+fp.Frame)
		}
		if strings.TrimSpace(fp.Caption) != "" {
			applied = append(applied, "подпись")
		}
	}

	final, err := encodePNG(img)
	if err != nil {
		return "", err
	}
	label := strings.Join(applied, ", ")
	if len([]rune(label)) > stylistLabelMax {
		label = string([]rune(label)[:stylistLabelMax])
	}
	id := editorSave(sessions, src, final, label)
	if id == 0 {
		return "", errors.New("не удалось сохранить результат в сессию приложения")
	}
	out := map[string]any{"id": id, "applied": applied, "photo": photoMeasures(img)}
	if src.item != nil {
		out["session_id"] = src.item.SessionID
		out["source_item"] = src.item.ID
	} else {
		out["session"] = EditorSessionName
	}
	out["note"] = editorNote
	for k, v := range notes {
		out[k] = v
	}
	return toJSON(out), nil
}

func registerEditorTools(s *Server) {
	s.Register(Tool{
		Name: "editor_describe",
		Description: "Фоторедактор: описать фото ЛОКАЛЬНОЙ vision-моделью (сцена, свет, цвета, настроение) и замерить его кодом: размер и ориентация, экспозиция, средняя яркость (0–255), контраст, насыщенность (0–1, средняя разница между самым ярким и самым тёмным каналом), цветовой оттенок, auto_suggest — автоуровни и температура как стартовые числа для adjust, face — где лицо (доли кадра; ищется через сервер). effects — эффекты, которые можно передать в editor_apply. " +
			"Фото — photo_path или item (\"active\" — открытое в приложении, или id элемента). Сам фото не открывай — фото не покидают локальную сеть; в ответе только текст.",
		InputSchema: props(map[string]any{
			"photo_path": prop("путь к фото (или item)", "string"),
			"item":       itemSchema,
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			eng, sessions, err := s.editorDeps()
			if err != nil {
				return "", err
			}
			src, err := readPhotoArg(args, sessions)
			if err != nil {
				return "", err
			}
			img, err := decodeImage(src.img)
			if err != nil {
				return "", err
			}
			description, err := eng.DescribeImage(src.img, editorDescribe)
			if err != nil {
				return "", fmt.Errorf("описание фото: %w", err)
			}
			face, _, _, _ := faceInfo(eng, src.img, img)
			groups, _, err := effectList(eng)
			if err != nil {
				return "", err
			}
			out := map[string]any{"description": description, "photo": photoMeasures(img), "face": face, "effects": groups}
			if src.item != nil {
				out["source_item"] = src.item.ID
			}
			return toJSON(out), nil
		},
	})

	ranges := make([]string, 0, len(adjust.Ranges))
	for k, r := range adjust.Ranges {
		ranges = append(ranges, fmt.Sprintf("%s %s…%s", k, strconv.FormatFloat(r[0], 'f', -1, 64), strconv.FormatFloat(r[1], 'f', -1, 64)))
	}
	sort.Strings(ranges)
	s.Register(Tool{
		Name: "editor_apply",
		Description: "Фоторедактор: применить к фото точные правки и сохранить результат в приложение (сессия «Фоторедактор» или та же сессия, что item). Порядок: effect → adjust → finish. " +
			"adjust — поправки числами, как ползунки приложения (0 — без изменений; white_point 255 и gamma 1 — нейтральные): " + strings.Join(ranges, ", ") + ". Лицо и людей не меняет. " +
			"effect — эффект постобработки по названию из editor_describe.effects и сила 0.2–0.9: перерисовка нейросетью flux-2-klein, 10–30 с; лицо может измениться — в ответе будет warning. " +
			"finish — кадр без нейросети: ratio (1:1, 4:5, 3:4, 9:16, 16:9, 3:2), fit (crop — обрезать; blur, white, black — вписать с полями), focus \"face\" (обрезать так, чтобы лицо осталось в кадре) или offset 0–1 (сдвиг вдоль обрезаемой стороны), frame (white, black, polaroid), caption (до 200 символов), caption_pos (bottom, top, center), caption_size (s, m, l); на полароиде подпись — на нижнем поле. " +
			"Каждый вызов берёт исходное фото — правки не накапливаются; нужен другой вариант — снова editor_apply с полным набором. Ответ: id элемента, applied, замеры результата. Картинок и путей в ответе нет.",
		InputSchema: props(map[string]any{
			"photo_path": prop("путь к фото (или item)", "string"),
			"item":       itemSchema,
			"adjust":     prop("поправки числами (ключи и диапазоны — в описании инструмента)", "object"),
			"effect": prop("эффект нейросетью", "object", map[string]any{"properties": map[string]any{
				"name": prop("название из editor_describe.effects", "string"), "strength": prop("0.2–0.9; по умолчанию — сила эффекта или 0.5", "number"),
			}}),
			"finish": prop("формат, рамка, подпись", "object", map[string]any{"properties": map[string]any{
				"ratio": prop("1:1, 4:5, 3:4, 9:16, 16:9, 3:2", "string"), "fit": prop("crop, blur, white, black", "string"),
				"focus": prop("face или center (для crop)", "string"), "offset": prop("0–1, сдвиг кадра для crop", "number"),
				"frame": prop("white, black, polaroid", "string"), "caption": prop("текст подписи", "string"),
				"caption_pos": prop("bottom, top, center", "string"), "caption_size": prop("s, m, l", "string"),
			}}),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			eng, sessions, err := s.editorDeps()
			if err != nil {
				return "", err
			}
			src, err := readPhotoArg(args, sessions)
			if err != nil {
				return "", err
			}
			return editorApply(eng, sessions, src, args)
		},
	})
}

//go:embed editor_prompt.md
var editorPromptText string

const editorPromptName = "photo_editor"

func editorPromptInfo() map[string]any {
	return map[string]any{
		"name":        editorPromptName,
		"description": "Фоторедактор: свет, цвет и настроение фото, формат под соцсети, рамка и подпись",
		"arguments": []map[string]any{{
			"name":        "photo",
			"description": "путь к фото или active — фото, открытое в приложении",
			"required":    false,
		}},
	}
}

func editorPrompt(photo string) map[string]any {
	text := editorPromptText
	switch p := strings.TrimSpace(photo); {
	case strings.EqualFold(p, "active"):
		text += "\nФото: item = \"active\" (открытое в приложении)."
	case p != "":
		text += "\nФото: photo_path = " + strconv.Quote(p) + "."
	}
	return map[string]any{
		"description": "Фоторедактор",
		"messages": []map[string]any{{
			"role":    "user",
			"content": map[string]any{"type": "text", "text": text},
		}},
	}
}
