package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"golang.org/x/net/http/httpproxy"
)

type StylistEngine interface {
	KleinEdit(imageBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error)
	DescribeImage(imageBase64, prompt string) (string, error)
	FaceLandmarks(imageBase64 string) ([]faceguard.Point, error)
}

type AppSessions interface {
	AddToNamedSession(name, imageBase64 string, info json.RawMessage, source string, isPreview bool, meta *preset.GenerationMeta) int64
}

type AppSessionItems interface {
	GetActiveSessionItem() (*preset.SessionItem, error)
	SessionItem(id int64) (*preset.SessionItem, error)
	GetSessionImage(id int64) (string, error)
	AddToSessionID(sessionID int64, imageBase64 string, info json.RawMessage, source string, isPreview bool, meta *preset.GenerationMeta) int64
}

type AppSessionFolders interface {
	ListSessions() ([]preset.SessionInfo, error)
	SessionItems(sessionID int64) ([]preset.SessionItem, error)
}

type photoSource struct {
	img  string
	item *preset.SessionItem
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
	Headwear  string `json:"headwear,omitempty"`
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
	stylistSkinLiftMax  = 5
	stylistMaxTries     = 3
	stylistBgShiftMax   = 30
	stylistTextureMin   = 0.1
	stylistTextureTie   = 0.05
	stylistFolderMax    = 6
	stylistKeepSuffix   = "Keep the person's face, identity, head position and angle, pose and background unchanged."
	stylistAskUser      = "Спроси у человека форму лица, возраст и тон кожи — по фото они определяются ненадёжно."
	stylistNote         = "Варианты — в приложении, сессия «Стилист»; человек открывает их там. Файлы не открывай: фото не покидают локальную сеть, результат описан в check."
	stylistFolderNote   = "Это разные снимки одного человека: устойчивое (волосы, стиль, черты) сведи в общий разбор, а то, что меняется от снимка к снимку (свет, тени под глазами), считай последствием съёмки. Примерять — stylist_try с item = id нужного фото; на нескольких фото — по вызову на каждое. Файлы не открывай."
	stylistItemNote     = "Варианты — в приложении, в той же сессии, что и исходное фото; человек открывает их там. Для уточнения снова передай item = source_item (не id варианта). Файлы не открывай: фото не покидают локальную сеть."
	stylistDescribe     = "Describe the person's current styling for a stylist, in short lines:\nHair: length (where it ends), cut, color, texture, parting.\nEyewear: none, or frame shape and color.\nClothing: each visible garment with type, color, material.\nAccessories: jewelry, hats, bags, or none.\nMakeup: lips and eyes, or none.\nBackground and lighting: one line.\nDescribe only what is visible. Do not guess age, face shape or skin tone."
	stylistNoStrands    = "No loose strands of hair across the cheeks or eyes."
	faceOriginal        = "original"
	faceNotProtected    = "not protected: "
	stylistCheck        = "Look at the person in this photo and describe their current styling in four short lines:\nHair: length (where it ends), cut name, color.\nEyewear: none, or frame shape/color and whether lenses are clear or tinted.\nOuter clothing: garment type, material, color.\nMakeup: lips and eyes.\nDescribe only what is visible."
)

var itemSchema = map[string]any{"type": "string", "description": "фото из приложения вместо photo_path: \"active\" — фото, открытое сейчас в приложении, или id элемента сессии строкой, например \"42\" (item из ответа stylist_describe или source_item); варианты кладутся в ту же сессию"}

var retouchSchema = prop("ретушь после правки (или без look — только ретушь): under_eyes — осветлить тени/круги под глазами, teeth — убрать желтизну зубов, eye_whites — убрать красноту белков, eye_color — новый цвет радужки (blue, light blue, green, grey, hazel, brown, amber, violet или #rrggbb), light — вытянуть уровни и осветлить тёмный кадр; face_width — ширина лица в % (−10…+10, минус — уже); только вместе с look: skin_tone \"original\" — вернуть исходный тон кожи (Klein часто осветляет), skin_lightness — кожа темнее/светлее в % (−30…+30, минус — темнее). Тон кожи меняется только по просьбе человека", "object",
	map[string]any{"properties": map[string]any{
		"under_eyes": prop("тени под глазами", "boolean"), "teeth": prop("зубы", "boolean"),
		"eye_whites": prop("белки глаз", "boolean"), "eye_color": prop("цвет глаз", "string"),
		"light":          prop("свет", "boolean"),
		"skin_tone":      prop("\"original\" — тон кожи как на исходном фото", "string"),
		"skin_lightness": prop("кожа темнее (минус) или светлее (плюс), %, −30…+30", "number"),
		"face_width":     prop("лицо уже (минус) или шире (плюс), %, −10…+10", "number"),
	}})

var lookSchema = prop("образ (по-английски): hair_cut, hair_color, outfit, headwear, glasses (\"none\" — снять; для очков пиши \"clear lenses\", иначе стёкла тонируются), makeup, free — любые поля", "object",
	map[string]any{"properties": map[string]any{
		"hair_cut": prop("стрижка", "string"), "hair_color": prop("цвет волос", "string"),
		"outfit": prop("одежда", "string"), "headwear": prop("головной убор", "string"),
		"glasses": prop("очки", "string"),
		"makeup":  prop("макияж", "string"), "free": prop("прочие изменения", "string"),
	}})

func buildLookInstruction(l Look) string {
	var parts []string
	add := func(format, v string) {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, fmt.Sprintf(format, strings.TrimRight(v, ". ")))
		}
	}
	add("Change the hairstyle to %s.", l.HairCut)
	if strings.TrimSpace(l.HairCut) != "" {
		parts = append(parts, stylistNoStrands)
	}
	add("Recolor the hair %s.", l.HairColor)
	add("Replace the current clothing with %s.", l.Outfit)
	add("Put on %s.", l.Headwear)
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
	for _, v := range []string{l.HairCut, l.HairColor, l.Outfit, l.Headwear, l.Glasses, l.Makeup, l.Free} {
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
		return nil, nil, errors.New("стилист недоступен в этой сборке imagecaster-mcp")
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

func readPhotoArg(args map[string]any, sessions AppSessions) (photoSource, error) {
	path := argString(args, "photo_path")
	raw, hasItem := args["item"]
	if hasItem && raw != nil {
		if path != "" {
			return photoSource{}, errors.New("укажи что-то одно: photo_path или item")
		}
		return readSessionPhoto(raw, sessions)
	}
	if path == "" {
		return photoSource{}, errors.New("нужен photo_path или item (\"active\" — фото, открытое в приложении, или id элемента сессии)")
	}
	img, err := filebrowser.ReadFileAsBase64(path)
	if err != nil {
		return photoSource{}, errors.New("не удалось прочитать фото по photo_path — проверь путь и формат")
	}
	return photoSource{img: img}, nil
}

func readSessionPhoto(raw any, sessions AppSessions) (photoSource, error) {
	items, ok := sessions.(AppSessionItems)
	if !ok {
		return photoSource{}, errors.New("фото из приложения недоступны в этой сборке imagecaster-mcp")
	}
	var item *preset.SessionItem
	var err error
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		if strings.EqualFold(v, "active") {
			item, err = items.GetActiveSessionItem()
			break
		}
		id, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil || id < 1 {
			return photoSource{}, errors.New("item: \"active\" или id элемента сессии")
		}
		item, err = items.SessionItem(id)
	case float64:
		if v < 1 || v != math.Trunc(v) {
			return photoSource{}, errors.New("item: id должен быть целым положительным числом")
		}
		item, err = items.SessionItem(int64(v))
	default:
		return photoSource{}, errors.New("item: \"active\" или числовой id элемента сессии")
	}
	if err != nil {
		return photoSource{}, errors.New("фото из приложения: не удалось прочитать элемент сессии")
	}
	if item == nil {
		return photoSource{}, errors.New("в приложении не выбрано фото (или такого элемента нет)")
	}
	if item.Source == "stylist" {
		if item.Meta != nil && item.Meta.SourceItem != 0 {
			return photoSource{}, fmt.Errorf("это вариант стилиста — правка поверх правки не делается; возьми исходное фото: item = %d", item.Meta.SourceItem)
		}
		return photoSource{}, errors.New("это вариант стилиста — правка поверх правки не делается; возьми исходное фото")
	}
	img, err := items.GetSessionImage(item.ID)
	if err != nil {
		return photoSource{}, errors.New("фото из приложения: файл элемента не прочитан")
	}
	return photoSource{img: img, item: item}, nil
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

type stylistAttempt struct {
	final, raw string
	info       json.RawMessage
	face       string
	fit        map[string]any
	editPts    []faceguard.Point
	pasted     bool
	bgShift    float64
	bgKnown    bool
	texture    float64
	texKnown   bool
}

func (a stylistAttempt) ok(protect bool) bool {
	return (!protect || a.pasted) && (!a.bgKnown || a.bgShift <= stylistBgShiftMax) && (!a.texKnown || a.texture >= stylistTextureMin)
}

func (a stylistAttempt) better(b stylistAttempt) bool {
	if a.pasted != b.pasted {
		return a.pasted
	}
	if a.texKnown && b.texKnown && math.Abs(a.texture-b.texture) > stylistTextureTie {
		return a.texture > b.texture
	}
	return a.bgShift < b.bgShift
}

func (a stylistAttempt) problem(protect bool) string {
	if protect && !a.pasted {
		return strings.TrimPrefix(a.face, faceNotProtected)
	}
	if a.texKnown && a.texture < stylistTextureMin {
		return fmt.Sprintf("лицо на правке не похоже на исходное по мелким деталям кожи (%.2f) — модель, похоже, нарисовала другого человека", a.texture)
	}
	return fmt.Sprintf("фон изменился (отличие %.0f) — модель правки, похоже, не опиралась на фото", a.bgShift)
}

func stylistJudge(eng StylistEngine, photo string, res *generation.GenerateImageResult, pts []faceguard.Point, face string) stylistAttempt {
	a := stylistAttempt{final: res.Image, raw: res.Image, info: res.Info, face: face}
	a.bgShift, a.bgKnown = faceguard.BackgroundShiftBase64(photo, res.Image, pts)
	if pts == nil {
		return a
	}
	editPts, err := eng.FaceLandmarks(res.Image)
	if err != nil {
		a.face = faceNotProtected + "на правке лицо не найдено: " + err.Error()
		return a
	}
	a.editPts = editPts
	if f, err := faceguard.MeasureBase64(res.Image, pts, editPts); err == nil {
		a.fit = map[string]any{"mean": round3(f.Mean), "key": round3(f.Key)}
	}
	if t, err := faceguard.TextureMatchBase64(photo, res.Image, pts, editPts); err == nil {
		a.texture, a.texKnown = t, true
	}
	guarded, err := faceguard.ApplyBase64(photo, res.Image, pts, editPts)
	if err != nil {
		a.face = faceNotProtected + "склейка не удалась: " + err.Error()
		return a
	}
	a.final, a.pasted = guarded, true
	return a
}

func stylistTry(eng StylistEngine, sessions AppSessions, src photoSource, look Look, retouch faceguard.Retouch, count int) (string, error) {
	photo := src.img
	if look == (Look{}) {
		return stylistRetouchOnly(eng, sessions, src, retouch)
	}
	instruction := buildLookInstruction(look)
	pts, face := faceLandmarksFor(eng, photo, look)
	protect := pts != nil
	meta := stylistMeta(src, lookLabel(look))
	var retouchNotes []string
	used := map[int64]bool{}
	var made []map[string]any
	var genErr error
	for range count {
		var best stylistAttempt
		tries := 0
		for tries < stylistMaxTries {
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
			a := stylistJudge(eng, photo, res, pts, face)
			if tries == 0 || a.better(best) {
				best = a
			}
			tries++
			if a.ok(protect) {
				break
			}
		}
		if tries == 0 {
			break
		}
		final := best.final
		skinNote := ""
		var finalPts []faceguard.Point
		if best.editPts != nil {
			finalPts = best.editPts
			final, skinNote = toneSkin(photo, final, pts, best.editPts, retouch)
		} else {
			why := strings.TrimPrefix(best.face, faceNotProtected)
			final, skinNote = skinWithoutFace(final, retouch, why)
		}
		if retouch.Local() || retouch.Light {
			var note string
			final, note = applyRetouch(eng, final, finalPts, retouch)
			if note != "" {
				retouchNotes = append(retouchNotes, note)
			}
		}
		id := saveVariant(sessions, src, final, best.info, meta)
		if id == 0 {
			genErr = errors.New("не удалось сохранить вариант в сессию приложения")
			break
		}
		check, err := eng.DescribeImage(final, stylistCheck)
		if err != nil {
			check = "не удалось описать результат: " + err.Error()
		}
		variant := map[string]any{"id": id, "check": check, "face": best.face}
		if best.fit != nil {
			variant["fit"] = best.fit
		}
		if skinNote != "" {
			variant["skin"] = skinNote
		}
		if tries > 1 {
			variant["attempts"] = tries
		}
		if !best.ok(protect) {
			variant["problem"] = best.problem(protect)
		}
		made = append(made, variant)
		if genErr != nil {
			break
		}
	}
	if len(made) == 0 {
		return "", fmt.Errorf("ни один вариант не получился: %w", genErr)
	}
	out := stylistOut(src)
	out["instruction"] = instruction
	out["variants"] = made
	out["face"] = face
	if len(retouchNotes) > 0 {
		out["retouch"] = retouchNotes[0]
	}
	if genErr != nil {
		out["failed"] = count - len(made)
		out["error"] = genErr.Error()
	}
	return toJSON(out), nil
}

func stylistMeta(src photoSource, label string) *preset.GenerationMeta {
	meta := &preset.GenerationMeta{Label: "Стилист: " + label}
	if src.item != nil {
		meta.SourceItem = src.item.ID
	}
	return meta
}

func stylistOut(src photoSource) map[string]any {
	if src.item == nil {
		return map[string]any{"session": StylistSessionName, "note": stylistNote}
	}
	return map[string]any{"session_id": src.item.SessionID, "source_item": src.item.ID, "note": stylistItemNote}
}

func saveVariant(sessions AppSessions, src photoSource, img string, info json.RawMessage, meta *preset.GenerationMeta) int64 {
	if src.item != nil {
		if items, ok := sessions.(AppSessionItems); ok {
			return items.AddToSessionID(src.item.SessionID, img, info, "stylist", false, meta)
		}
		return 0
	}
	return sessions.AddToNamedSession(StylistSessionName, img, info, "stylist", false, meta)
}

func applyRetouch(eng StylistEngine, img string, pts []faceguard.Point, r faceguard.Retouch) (string, string) {
	note := ""
	if r.Local() && pts == nil {
		p, err := eng.FaceLandmarks(img)
		switch {
		case err == nil:
			pts = p
		case r.Light:
			r, note = faceguard.Retouch{Light: true}, "сделан только свет: лицо не найдено"
		default:
			return img, "ретушь не сделана: лицо не найдено"
		}
	}
	out, err := faceguard.ApplyRetouchBase64(img, pts, r)
	if err != nil {
		return img, "ретушь не сделана: " + err.Error()
	}
	return out, note
}

func toneSkin(photo, img string, origPts, editPts []faceguard.Point, r faceguard.Retouch) (string, string) {
	if r.Skin() {
		out, err := faceguard.ToneSkinBase64(img, photo, editPts, origPts, r.SkinTone == faceguard.SkinToneOriginal, r.SkinLightness)
		if err != nil {
			return img, "тон кожи не изменён: " + err.Error()
		}
		img = out
	}
	if r.SkinTone == faceguard.SkinToneOriginal {
		return img, ""
	}
	lift, err := faceguard.SkinLiftBase64(photo, img, origPts, editPts)
	if err != nil || lift <= stylistSkinLiftMax {
		return img, ""
	}
	return img, fmt.Sprintf("кожа светлее исходной на %d%% — retouch.skin_tone=\"original\" вернёт тон", int(math.Round(lift)))
}

func skinWithoutFace(img string, r faceguard.Retouch, why string) (string, string) {
	note := ""
	if r.SkinTone == faceguard.SkinToneOriginal {
		note = "исходный тон кожи не возвращён: " + why
	}
	if r.SkinLightness != 0 {
		out, err := faceguard.ToneSkinBase64(img, img, nil, nil, false, r.SkinLightness)
		if err != nil {
			return img, "тон кожи не изменён: " + err.Error()
		}
		img = out
	}
	return img, note
}

func retouchLabel(r faceguard.Retouch) string {
	var parts []string
	if r.UnderEyes {
		parts = append(parts, "тени под глазами")
	}
	if r.Teeth {
		parts = append(parts, "зубы")
	}
	if r.EyeWhites {
		parts = append(parts, "белки глаз")
	}
	if c := strings.TrimSpace(r.EyeColor); c != "" {
		parts = append(parts, "цвет глаз "+c)
	}
	if r.Light {
		parts = append(parts, "свет")
	}
	if r.FaceWidth != 0 {
		parts = append(parts, fmt.Sprintf("ширина лица %+.0f%%", r.FaceWidth))
	}
	return "ретушь: " + strings.Join(parts, ", ")
}

func stylistRetouchOnly(eng StylistEngine, sessions AppSessions, src photoSource, r faceguard.Retouch) (string, error) {
	final, note := applyRetouch(eng, src.img, nil, r)
	if final == src.img {
		if note == "" {
			note = "ретушь ничего не изменила: на фото нечего исправлять"
		}
		return "", errors.New(note)
	}
	id := saveVariant(sessions, src, final, nil, stylistMeta(src, retouchLabel(r)))
	if id == 0 {
		return "", errors.New("не удалось сохранить результат в сессию приложения")
	}
	out := stylistOut(src)
	out["variants"] = []map[string]any{{"id": id}}
	if note != "" {
		out["retouch"] = note
	}
	return toJSON(out), nil
}

func findSession(folders AppSessionFolders, name string) (*preset.SessionInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("session: укажи название папки истории")
	}
	all, err := folders.ListSessions()
	if err != nil {
		return nil, errors.New("не удалось прочитать папки истории приложения")
	}
	var partial []preset.SessionInfo
	for i := range all {
		if strings.EqualFold(strings.TrimSpace(all[i].Name), name) {
			return &all[i], nil
		}
		if strings.Contains(strings.ToLower(all[i].Name), strings.ToLower(name)) {
			partial = append(partial, all[i])
		}
	}
	if len(partial) == 1 {
		return &partial[0], nil
	}
	names := make([]string, 0, len(all))
	src := all
	if len(partial) > 1 {
		src = partial
	}
	for _, si := range src {
		names = append(names, "«"+si.Name+"»")
	}
	if len(partial) > 1 {
		return nil, fmt.Errorf("под «%s» подходят несколько папок: %s — уточни название", name, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("папки истории «%s» нет; есть: %s", name, strings.Join(names, ", "))
}

func describeFolder(eng StylistEngine, sessions AppSessions, name string) (string, error) {
	folders, ok := sessions.(AppSessionFolders)
	items, ok2 := sessions.(AppSessionItems)
	if !ok || !ok2 {
		return "", errors.New("папки истории недоступны в этой сборке imagecaster-mcp")
	}
	si, err := findSession(folders, name)
	if err != nil {
		return "", err
	}
	all, err := folders.SessionItems(si.ID)
	if err != nil {
		return "", errors.New("не удалось прочитать фото папки истории")
	}
	var originals []preset.SessionItem
	variants := 0
	for _, it := range all {
		switch {
		case it.Source == "stylist":
			variants++
		case !it.IsPreview:
			originals = append(originals, it)
		}
	}
	if len(originals) == 0 {
		return "", fmt.Errorf("в папке «%s» нет исходных фото (вариантов стилиста: %d)", si.Name, variants)
	}
	more := 0
	if len(originals) > stylistFolderMax {
		more = len(originals) - stylistFolderMax
		originals = originals[len(originals)-stylistFolderMax:]
	}
	photos := make([]map[string]any, 0, len(originals))
	for _, it := range originals {
		entry := map[string]any{"item": it.ID, "created_at": it.CreatedAt}
		img, err := items.GetSessionImage(it.ID)
		if err != nil {
			entry["error"] = "файл не прочитан"
			photos = append(photos, entry)
			continue
		}
		if d, err := eng.DescribeImage(img, stylistDescribe); err != nil {
			entry["error"] = "описание не получилось"
		} else {
			entry["description"] = d
		}
		if p := describePhoto(eng, img); p != "" {
			entry["photo"] = p
		}
		photos = append(photos, entry)
	}
	out := map[string]any{
		"session":    si.Name,
		"session_id": si.ID,
		"photos":     photos,
		"ask_user":   stylistAskUser,
		"note":       stylistFolderNote,
	}
	if variants > 0 {
		out["skipped_variants"] = variants
	}
	if more > 0 {
		out["older_not_described"] = more
	}
	return toJSON(out), nil
}

func describePhoto(eng StylistEngine, photo string) string {
	pts, err := eng.FaceLandmarks(photo)
	if err != nil {
		pts = nil
	}
	st, err := faceguard.AnalyzeBase64(photo, pts)
	if err != nil {
		return ""
	}
	var lines []string
	switch {
	case st.Gamma > 1.15:
		lines = append(lines, "кадр темноват — поможет retouch.light")
	case st.Gamma < 0.87:
		lines = append(lines, "кадр светлый")
	default:
		lines = append(lines, "экспозиция нормальная")
	}
	if math.Abs(st.Temperature) >= 50 {
		lines = append(lines, "возможен цветной оттенок от света (автоматически не правится)")
	}
	if pts == nil {
		return strings.Join(lines, "; ") + "; лицо не найдено — тени под глазами не измерены"
	}
	pct := int(math.Round(st.UnderEye * 100))
	var level string
	switch {
	case st.UnderEye < 0.08:
		level = "почти нет"
	case st.UnderEye < 0.15:
		level = "лёгкие"
	case st.UnderEye < 0.25:
		level = "заметные — после макияжа станут контрастнее, предложи retouch.under_eyes"
	default:
		level = "сильные — предложи retouch.under_eyes"
	}
	lines = append(lines, fmt.Sprintf("тени под глазами: %s (зона под глазами темнее щеки на %d%%)", level, pct))
	return strings.Join(lines, "; ")
}

func faceLandmarksFor(eng StylistEngine, photo string, look Look) ([]faceguard.Point, string) {
	if strings.TrimSpace(look.Glasses) != "" {
		return nil, faceNotProtected + "с очками лицо не вклеивается — очки легли бы под исходное лицо"
	}
	pts, err := eng.FaceLandmarks(photo)
	if err != nil {
		return nil, faceNotProtected + "лицо на фото не найдено: " + err.Error()
	}
	return pts, faceOriginal
}

func registerStylistTools(s *Server) {
	s.Register(Tool{
		Name: "stylist_describe",
		Description: "Стилист: описать внешний вид человека на фото ЛОКАЛЬНОЙ vision-моделью — в ответе только текст. " +
			"Фото — photo_path, item (\"active\" — открытое в приложении, или id) или session — папка истории приложения по названию: тогда описываются все её исходные фото (до 6 последних), у каждого id для stylist_try. Поле photo — замеры по пикселям: экспозиция и насколько тени под глазами темнее щеки; по ним решай, нужна ли retouch. " +
			"Сам фото не открывай и не читай — фото не должны покидать локальную сеть. Форму лица, возраст и тон кожи спроси у человека.",
		InputSchema: props(map[string]any{
			"photo_path": prop("путь к фото человека (или item, или session)", "string"),
			"item":       itemSchema,
			"session":    prop("название папки истории в приложении (можно часть, регистр не важен) — описать все её исходные фото (до 6 последних)", "string"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			eng, sessions, err := s.stylistDeps()
			if err != nil {
				return "", err
			}
			if name := argString(args, "session"); name != "" {
				if argString(args, "photo_path") != "" || args["item"] != nil {
					return "", errors.New("укажи что-то одно: session, photo_path или item")
				}
				return describeFolder(eng, sessions, name)
			}
			src, err := readPhotoArg(args, sessions)
			if err != nil {
				return "", err
			}
			description, err := eng.DescribeImage(src.img, stylistDescribe)
			if err != nil {
				return "", fmt.Errorf("описание фото: %w", err)
			}
			out := map[string]any{"description": description, "ask_user": stylistAskUser}
			if photo := describePhoto(eng, src.img); photo != "" {
				out["photo"] = photo
			}
			if src.item != nil {
				out["source_item"] = src.item.ID
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "stylist_try",
		Description: "Стилист: примерить образ на фото. Каждый вариант — одна правка ИСХОДНОГО фото flux-2-klein (держит позу и поворот головы), 10–30 с на вариант; варианты сразу появляются в приложении в сессии «Стилист». " +
			"Фото — photo_path (варианты в сессию «Стилист») или item: \"active\" — открытое в приложении, либо id элемента (варианты в ту же сессию). " +
			"Варианты различаются сидом (слабо) — разные идеи давай разными вызовами. Уточнение — снова stylist_try с тем же photo_path или item = source_item и полным изменённым look (правка поверх правки не делается). " +
			"retouch — честная ретушь после правки: тени под глазами, зубы, белки, цвет глаз, свет, ширина лица (face_width); тон кожи (skin_tone=\"original\" — как на исходном фото, skin_lightness — темнее/светлее) — только по просьбе человека и только вместе с look; без look — только ретушь, один вариант, без генерации. " +
			"Черты лица (глаза, нос, рот) после правки вклеиваются с исходного фото по точкам лица в тоне правки, кожа лица — от правки; с очками (glasses) лицо не вклеивается — поле face в ответе говорит, защищено ли лицо. Если лицо не совпало (модель повернула голову), на правке другой человек (не совпали мелкие детали кожи) или фон сменился (модель не опиралась на фото), вариант перегенерируется, до 3 попыток (поле attempts); не вышло — поле problem, скажи человеку. Поле skin у варианта — правка заметно осветлила кожу: скажи человеку и предложи retouch.skin_tone=\"original\". " +
			"Ответ: id элементов сессии и check — короткое описание результата локальной vision-моделью (тонировку стёкол не различает). Картинок и путей в ответе нет.",
		InputSchema: props(map[string]any{
			"photo_path": prop("путь к исходному фото человека (или item)", "string"),
			"item":       itemSchema,
			"look":       lookSchema,
			"retouch":    retouchSchema,
			"variants":   prop("сколько вариантов 1–4 (по умолчанию 2); при одной ретуши без look — всегда 1", "integer"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			eng, sessions, err := s.stylistDeps()
			if err != nil {
				return "", err
			}
			look, err := decodeArg[Look](args, "look")
			if err != nil {
				return "", err
			}
			retouch, err := decodeArg[faceguard.Retouch](args, "retouch")
			if err != nil {
				return "", err
			}
			if look == nil {
				look = &Look{}
			}
			if retouch == nil {
				retouch = &faceguard.Retouch{}
			}
			if *look == (Look{}) && retouch.IsZero() {
				return "", errors.New("look и retouch пусты — укажи хотя бы одно изменение")
			}
			if err := retouch.Validate(); err != nil {
				return "", err
			}
			if *look == (Look{}) && retouch.Skin() {
				return "", errors.New("skin_tone и skin_lightness — только вместе с look: тон кожи меняет правка Klein")
			}
			if c := strings.TrimSpace(retouch.EyeColor); c != "" {
				if _, err := faceguard.EyeColorRGB(c); err != nil {
					return "", err
				}
			}
			count, err := variantCount(args)
			if err != nil {
				return "", err
			}
			src, err := readPhotoArg(args, sessions)
			if err != nil {
				return "", err
			}
			return stylistTry(eng, sessions, src, *look, *retouch, count)
		},
	})
}

//go:embed stylist_prompt.md
var stylistPromptText string

const stylistPromptName = "stylist"

func stylistPromptInfo() map[string]any {
	return map[string]any{
		"name":        stylistPromptName,
		"description": "Стилист: подбор образа под внешность и честная ретушь фото",
		"arguments": []map[string]any{{
			"name":        "photo",
			"description": "путь к фото или active — фото, открытое в приложении",
			"required":    false,
		}},
	}
}

func stylistPrompt(photo string) map[string]any {
	text := stylistPromptText
	switch p := strings.TrimSpace(photo); {
	case strings.EqualFold(p, "active"):
		text += "\nФото: item = \"active\" (открытое в приложении)."
	case p != "":
		text += "\nФото: photo_path = " + strconv.Quote(p) + "."
	}
	return map[string]any{
		"description": "Стилист",
		"messages": []map[string]any{{
			"role":    "user",
			"content": map[string]any{"type": "text", "text": text},
		}},
	}
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
