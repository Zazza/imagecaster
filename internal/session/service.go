package session

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/imgformat"
	"github.com/Zazza/imagecaster/internal/preset"

	xdraw "golang.org/x/image/draw"
)

type EventEmitter interface {
	Emit(event string, data ...any)
}

type Service struct {
	db      *preset.DB
	dataDir string
	emit    EventEmitter
	moveMu  sync.Mutex
}

func New(db *preset.DB, dataDir string, emit EventEmitter) *Service {
	return &Service{db: db, dataDir: dataDir, emit: emit}
}

type SDInfo struct {
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt"`
	Sampler        string  `json:"sampler"`
	SamplerName    string  `json:"sampler_name"`
	Steps          int     `json:"steps"`
	CfgScale       float64 `json:"cfg_scale"`
	Seed           int64   `json:"seed"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Denoising      float64 `json:"denoising_strength"`
	SDModelName    string  `json:"sd_model_name"`
}

const MaxStoredImageBase64 = 50 * 1024 * 1024

func unwrapInfo(info json.RawMessage) []byte {
	if len(info) == 0 {
		return nil
	}
	var inner string
	if json.Unmarshal(info, &inner) == nil {
		return []byte(inner)
	}
	return info
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncateMetaStr(s string) string {
	if len(s) <= preset.MaxMetaStrLen {
		return s
	}
	cut := s[:preset.MaxMetaStrLen]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

func metaJSONLen(m *preset.GenerationMeta) int {
	b, err := json.Marshal(m)
	if err != nil {
		return preset.MaxMetaJSONLen + 1
	}
	return len(b)
}

func normalizeMeta(meta *preset.GenerationMeta, sdModel string) *preset.GenerationMeta {
	if meta == nil {
		return nil
	}
	m := &preset.GenerationMeta{
		PresetID:   meta.PresetID,
		Model:      truncateMetaStr(firstNonEmpty(sdModel, meta.Model)),
		Label:      truncateMetaStr(meta.Label),
		SourceItem: meta.SourceItem,
	}
	if len(meta.Chain) > 0 {
		n := len(meta.Chain)
		if n > preset.MaxMetaChainSteps {
			n = preset.MaxMetaChainSteps
		}
		m.Chain = make([]preset.StepMeta, 0, n)
		for i := 0; i < n; i++ {
			st := meta.Chain[i]
			sm := preset.StepMeta{
				Step:    st.Step,
				Preset:  truncateMetaStr(st.Preset),
				Op:      truncateMetaStr(st.Op),
				Model:   truncateMetaStr(st.Model),
				Denoise: st.Denoise,
			}
			if len(st.Loras) <= preset.MaxStepLorasLen {
				sm.Loras = st.Loras
			}
			m.Chain = append(m.Chain, sm)
		}
	}
	fullLen := metaJSONLen(m)
	if fullLen <= preset.MaxMetaJSONLen {
		return m
	}
	for i := range m.Chain {
		m.Chain[i].Loras = ""
	}
	if metaJSONLen(m) <= preset.MaxMetaJSONLen {
		return m
	}
	m.Chain = nil
	if metaJSONLen(m) <= preset.MaxMetaJSONLen {
		return m
	}
	log.Printf("[session] warn: generation meta dropped: json=%d bytes over cap=%d bytes", fullLen, preset.MaxMetaJSONLen)
	return nil
}

func (s *Service) AddToSession(imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64 {
	sessionID, err := s.db.GetActiveSessionID()
	if err != nil {
		log.Printf("[session] read active session: %v", err)
		return 0
	}
	if sessionID == 0 {
		log.Printf("[session] no active session, %s image not saved", source)
		return 0
	}
	return s.addItem(sessionID, imageBase64, info, source, meta)
}

type pinnedSessions struct {
	svc       *Service
	sessionID int64
}

func (s *Service) PinnedTo(sessionID int64) generation.SessionAdder {
	return pinnedSessions{svc: s, sessionID: sessionID}
}

func (p pinnedSessions) AddToSession(imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64 {
	p.svc.moveMu.Lock()
	exists, err := p.svc.db.SessionExists(p.sessionID)
	var itemID int64
	var storeErr error
	if err == nil && exists {
		itemID, storeErr = p.svc.storeItem(p.sessionID, imageBase64, info, source, meta)
	}
	p.svc.moveMu.Unlock()
	if err != nil || !exists {
		log.Printf("[session] session %d of the job is gone (err=%v), %s image goes to the active session", p.sessionID, err, source)
		return p.svc.AddToSession(imageBase64, info, source, meta)
	}
	if storeErr != nil {
		log.Printf("[session] %s image not saved to session %d: %v", source, p.sessionID, storeErr)
		return 0
	}
	p.svc.emitAdded(itemID, p.sessionID)
	return itemID
}

func (s *Service) AddToNamedSession(name, imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64 {
	sessionID, err := s.db.FindSessionByName(name)
	if err != nil {
		log.Printf("[session] find session %q: %v", name, err)
		return 0
	}
	if sessionID == 0 {
		si, err := s.db.CreateSession(name)
		if err != nil {
			log.Printf("[session] create session %q: %v", name, err)
			return 0
		}
		sessionID = si.ID
		s.emit.Emit("session:created", map[string]any{"session_id": si.ID, "name": si.Name})
	}
	return s.addItem(sessionID, imageBase64, info, source, meta)
}

func (s *Service) AddToSessionID(sessionID int64, imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64 {
	if sessionID == 0 {
		log.Printf("[session] no session id, %s image not saved", source)
		return 0
	}
	return s.addItem(sessionID, imageBase64, info, source, meta)
}

func (s *Service) SessionItems(sessionID int64) ([]preset.SessionItem, error) {
	return s.db.GetSessionItems(sessionID)
}

func (s *Service) SessionItem(id int64) (*preset.SessionItem, error) {
	return s.db.GetSessionItem(id)
}

func (s *Service) sessionDir(sessionID int64) string {
	return filepath.Join(s.dataDir, "sessions", strconv.FormatInt(sessionID, 10))
}

func (s *Service) thumbDir(sessionID int64) string {
	return filepath.Join(s.dataDir, "thumbs", strconv.FormatInt(sessionID, 10))
}

func (s *Service) addItem(sessionID int64, imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64 {
	itemID, err := s.storeItem(sessionID, imageBase64, info, source, meta)
	if err != nil {
		log.Printf("[session] %s image not saved to session %d: %v", source, sessionID, err)
		return 0
	}
	s.emitAdded(itemID, sessionID)
	return itemID
}

func (s *Service) emitAdded(itemID, sessionID int64) {
	s.emit.Emit("session:added", map[string]int64{"id": itemID, "session_id": sessionID})
	s.emit.Emit("session:active", map[string]int64{"id": itemID})
}

type encodedImage struct {
	data   []byte
	ext    string
	width  int
	height int
}

func decodeStoredImage(imageBase64 string) (*encodedImage, error) {
	if len(imageBase64) > MaxStoredImageBase64 {
		data, err := reencodeJPEG(imageBase64)
		if err != nil {
			return nil, fmt.Errorf("oversized image: base64=%d bytes, cap=%d bytes, jpeg reencode failed: %w", len(imageBase64), MaxStoredImageBase64, err)
		}
		if base64.StdEncoding.EncodedLen(len(data)) > MaxStoredImageBase64 {
			return nil, fmt.Errorf("oversized image: base64=%d bytes, jpeg=%d bytes still over cap=%d bytes", len(imageBase64), base64.StdEncoding.EncodedLen(len(data)), MaxStoredImageBase64)
		}
		return describeImage(data)
	}
	data, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	return describeImage(data)
}

func describeImage(data []byte) (*encodedImage, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image header: %w", err)
	}
	switch format {
	case "png":
		return &encodedImage{data: data, ext: ".png", width: cfg.Width, height: cfg.Height}, nil
	case "jpeg":
		if o := imgformat.JPEGOrientation(data); o != 1 {
			return uprightJPEG(data, o)
		}
		return &encodedImage{data: data, ext: ".jpg", width: cfg.Width, height: cfg.Height}, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode %s image: %w", format, err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode %s image as png: %w", format, err)
	}
	return &encodedImage{data: buf.Bytes(), ext: ".png", width: img.Bounds().Dx(), height: img.Bounds().Dy()}, nil
}

func uprightJPEG(data []byte, orientation int) (*encodedImage, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode jpeg image: %w", err)
	}
	img = imgformat.ApplyOrientation(img, orientation)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, fmt.Errorf("encode upright jpeg: %w", err)
	}
	return &encodedImage{data: buf.Bytes(), ext: ".jpg", width: img.Bounds().Dx(), height: img.Bounds().Dy()}, nil
}

const thumbSize = 128

func makeThumb(img *encodedImage) ([]byte, string, error) {
	if img.width <= thumbSize && img.height <= thumbSize {
		return img.data, img.ext, nil
	}
	src, _, err := image.Decode(bytes.NewReader(img.data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	origW := src.Bounds().Dx()
	origH := src.Bounds().Dy()
	ratio := math.Min(float64(thumbSize)/float64(origW), float64(thumbSize)/float64(origH))
	tw := max(int(float64(origW)*ratio), 1)
	th := max(int(float64(origH)*ratio), 1)
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, "", fmt.Errorf("encode thumbnail: %w", err)
	}
	return buf.Bytes(), ".jpg", nil
}

const (
	storedDirMode  = os.ModeSetgid | 0o770
	storedFileMode = 0o640
)

func ensureStoredDir(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if parent := filepath.Dir(dir); parent != dir {
		if err := ensureStoredDir(parent); err != nil {
			return err
		}
	}
	if err := os.Mkdir(dir, 0o770); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	return os.Chmod(dir, storedDirMode)
}

func writeTemp(dir string, data []byte) (string, error) {
	if err := ensureStoredDir(dir); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	werr := f.Chmod(storedFileMode)
	if werr == nil {
		_, werr = f.Write(data)
	}
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		removeQuiet(f.Name())
		return "", werr
	}
	return f.Name(), nil
}

func removeQuiet(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("[session] remove %s: %v", path, err)
	}
}

func (s *Service) storeItem(sessionID int64, imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) (int64, error) {
	img, err := decodeStoredImage(imageBase64)
	if err != nil {
		return 0, err
	}
	thumbData, thumbExt, err := makeThumb(img)
	if err != nil {
		return 0, err
	}

	item := &preset.SessionItem{
		SessionID: sessionID,
		Source:    source,
		Width:     img.width,
		Height:    img.height,
	}
	var sd SDInfo
	if raw := unwrapInfo(info); raw != nil {
		if json.Unmarshal(raw, &sd) == nil {
			item.Prompt = sd.Prompt
			item.NegativePrompt = sd.NegativePrompt
			item.Sampler = sd.Sampler
			if item.Sampler == "" {
				item.Sampler = sd.SamplerName
			}
			item.Steps = sd.Steps
			item.CfgScale = sd.CfgScale
			item.Seed = &sd.Seed
			item.Denoising = sd.Denoising
			if sd.Width > 0 {
				item.Width = sd.Width
			}
			if sd.Height > 0 {
				item.Height = sd.Height
			}
		}
	}
	item.Meta = normalizeMeta(meta, sd.SDModelName)
	if meta != nil {
		item.PresetID = meta.PresetID
	}

	sessionDir := s.sessionDir(sessionID)
	thumbDir := s.thumbDir(sessionID)
	tmpFile, err := writeTemp(sessionDir, img.data)
	if err != nil {
		return 0, fmt.Errorf("write image: %w", err)
	}
	defer removeQuiet(tmpFile)
	tmpThumb, err := writeTemp(thumbDir, thumbData)
	if err != nil {
		return 0, fmt.Errorf("write thumbnail: %w", err)
	}
	defer removeQuiet(tmpThumb)

	var placed []string
	itemID, err := s.db.AddSessionItemWithFiles(item, func(id int64) (string, string, error) {
		fileName := strconv.FormatInt(id, 10) + img.ext
		thumbName := strconv.FormatInt(id, 10) + thumbExt
		filePath := filepath.Join(sessionDir, fileName)
		if err := os.Rename(tmpFile, filePath); err != nil {
			return "", "", fmt.Errorf("place image: %w", err)
		}
		placed = append(placed, filePath)
		thumbPath := filepath.Join(thumbDir, thumbName)
		if err := os.Rename(tmpThumb, thumbPath); err != nil {
			return "", "", fmt.Errorf("place thumbnail: %w", err)
		}
		placed = append(placed, thumbPath)
		return fileName, thumbName, nil
	})
	if err != nil {
		for _, p := range placed {
			removeQuiet(p)
		}
		return 0, err
	}
	return itemID, nil
}

func reencodeJPEG(imageBase64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	img = imgformat.ApplyOrientation(img, imgformat.JPEGOrientation(data))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Service) CreateSession(name string) (*preset.SessionInfo, error) {
	si, err := s.db.CreateSession(name)
	if err != nil {
		return nil, err
	}
	if err := s.db.SetActiveSession(si.ID); err != nil {
		return nil, err
	}
	s.emit.Emit("session:created", map[string]any{"session_id": si.ID, "name": si.Name})
	s.emit.Emit("session:switched", map[string]int64{"session_id": si.ID})
	return si, nil
}

func (s *Service) ListHistory(f preset.HistoryFilter) (*preset.HistoryPage, error) {
	return s.db.ListHistory(f)
}

func (s *Service) OpenInHistory(id int64) error {
	item, err := s.db.GetSessionItem(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("session item not found")
	}
	if err := s.SwitchSession(item.SessionID); err != nil {
		return err
	}
	return s.SetActiveSessionItem(id)
}

func (s *Service) ListSessions() ([]preset.SessionInfo, error) {
	sessions, err := s.db.ListSessions()
	if err != nil {
		return nil, err
	}
	activeID, err := s.db.GetActiveSessionID()
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		sessions[i].IsActive = activeID != 0 && sessions[i].ID == activeID
	}
	return sessions, nil
}

func (s *Service) SwitchSession(id int64) error {
	if err := s.db.SetActiveSession(id); err != nil {
		return err
	}
	s.emit.Emit("session:switched", map[string]int64{"session_id": id})
	return nil
}

func (s *Service) RenameSession(id int64, name string) error {
	return s.db.RenameSession(id, name)
}

func (s *Service) DeleteSession(id int64) error {
	s.moveMu.Lock()
	defer s.moveMu.Unlock()
	sessions, err := s.db.ListSessions()
	if err != nil {
		return err
	}
	if len(sessions) <= 1 {
		return fmt.Errorf("cannot delete the last session")
	}

	activeID, err := s.db.GetActiveSessionID()
	if err != nil {
		return err
	}
	if activeID == id {
		for _, si := range sessions {
			if si.ID != id {
				if err := s.db.SetActiveSession(si.ID); err != nil {
					return fmt.Errorf("switch active session: %w", err)
				}
				break
			}
		}
	}
	if err := s.db.DeleteSession(id); err != nil {
		return err
	}
	s.removeSessionFiles(id)

	s.emit.Emit("session:deleted", map[string]int64{"session_id": id})
	return nil
}

func (s *Service) removeSessionFiles(sessionID int64) {
	for _, dir := range []string{s.sessionDir(sessionID), s.thumbDir(sessionID)} {
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("[session] remove %s: %v", dir, err)
		}
	}
}

func (s *Service) GetSessionItems() ([]preset.SessionItem, error) {
	sessionID, err := s.db.GetActiveSessionID()
	if err != nil {
		return nil, err
	}
	if sessionID == 0 {
		return []preset.SessionItem{}, nil
	}
	return s.db.GetSessionItems(sessionID)
}

func (s *Service) GetActiveSessionItem() (*preset.SessionItem, error) {
	sessionID, err := s.db.GetActiveSessionID()
	if err != nil {
		return nil, err
	}
	if sessionID == 0 {
		return nil, nil
	}
	return s.db.GetActiveItem(sessionID)
}

func (s *Service) SetActiveSessionItem(id int64) error {
	sessionID, err := s.db.GetActiveSessionID()
	if err != nil {
		return err
	}
	if err := s.db.SetActiveItem(id, sessionID); err != nil {
		return err
	}
	s.emit.Emit("session:active", map[string]int64{"id": id})
	s.emit.Emit("session:selected", map[string]int64{"id": id})
	return nil
}

func (s *Service) DeleteSessionItem(id int64) error {
	deleted, err := s.removeSessionItem(id)
	if err != nil || !deleted {
		return err
	}
	s.emit.Emit("session:removed", map[string]int64{"id": id})
	return nil
}

func (s *Service) DeleteSessionItems(ids []int64) (int, error) {
	n := 0
	var firstErr error
	for _, id := range ids {
		deleted, err := s.removeSessionItem(id)
		if err != nil {
			log.Printf("[session] delete item %d: %v", id, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if deleted {
			n++
		}
	}
	if n > 0 {
		s.emit.Emit("session:removed", map[string]any{"ids": ids})
	}
	return n, firstErr
}

func (s *Service) MoveSessionItems(ids []int64, sessionID int64) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("не выбрано ни одного фото")
	}
	s.moveMu.Lock()
	defer s.moveMu.Unlock()
	ok, err := s.db.SessionExists(sessionID)
	if err != nil {
		return 0, err
	}
	if sessionID <= 0 || !ok {
		return 0, errors.New("папки истории нет")
	}
	var items []*preset.SessionItem
	seen := map[int64]bool{}
	sameFolder := false
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		item, err := s.db.GetSessionItem(id)
		if err != nil {
			return 0, err
		}
		if item == nil {
			continue
		}
		if item.SessionID == sessionID {
			sameFolder = true
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		if sameFolder {
			return 0, errors.New("фото уже в этой папке")
		}
		return 0, nil
	}
	if err := ensureStoredDir(s.sessionDir(sessionID)); err != nil {
		return 0, fmt.Errorf("папка истории не создана: %w", err)
	}
	if err := ensureStoredDir(s.thumbDir(sessionID)); err != nil {
		return 0, fmt.Errorf("папка миниатюр не создана: %w", err)
	}
	n := 0
	var moved []int64
	for _, item := range items {
		if err := s.moveItem(item, sessionID); err != nil {
			log.Printf("[session] move item %d to session %d: %v", item.ID, sessionID, err)
			if n > 0 {
				s.emit.Emit("session:removed", map[string]any{"ids": moved, "moved_to": sessionID})
			}
			if n > 0 {
				return n, fmt.Errorf("перенесено %d из %d, фото %d не перенесено: %w", n, len(items), item.ID, err)
			}
			return n, fmt.Errorf("фото %d не перенесено: %w", item.ID, err)
		}
		n++
		moved = append(moved, item.ID)
	}
	s.emit.Emit("session:removed", map[string]any{"ids": moved, "moved_to": sessionID})
	return n, nil
}

func (s *Service) moveItem(item *preset.SessionItem, sessionID int64) error {
	type move struct{ from, to string }
	var done []move
	undo := func() {
		for i := len(done) - 1; i >= 0; i-- {
			if err := os.Rename(done[i].to, done[i].from); err != nil {
				log.Printf("[session] move back %s: %v", done[i].to, err)
			}
		}
	}
	plan := []move{}
	if item.FileName != "" {
		plan = append(plan, move{filepath.Join(s.sessionDir(item.SessionID), item.FileName), filepath.Join(s.sessionDir(sessionID), item.FileName)})
	}
	if item.ThumbName != "" {
		plan = append(plan, move{filepath.Join(s.thumbDir(item.SessionID), item.ThumbName), filepath.Join(s.thumbDir(sessionID), item.ThumbName)})
	}
	for _, m := range plan {
		if _, err := os.Stat(m.from); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(m.to); err == nil {
			undo()
			return fmt.Errorf("в папке назначения уже есть %s", filepath.Base(m.to))
		}
		if err := os.Rename(m.from, m.to); err != nil {
			undo()
			return err
		}
		done = append(done, m)
	}
	if err := s.db.MoveSessionItem(item.ID, sessionID); err != nil {
		undo()
		return err
	}
	return nil
}

func (s *Service) removeSessionItem(id int64) (bool, error) {
	item, err := s.db.GetSessionItem(id)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}
	if err := s.db.DeleteSessionItem(id); err != nil {
		return false, err
	}
	if item.FileName != "" {
		removeQuiet(filepath.Join(s.sessionDir(item.SessionID), item.FileName))
	}
	if item.ThumbName != "" {
		removeQuiet(filepath.Join(s.thumbDir(item.SessionID), item.ThumbName))
	}
	return true, nil
}

func (s *Service) ClearSession() error {
	s.moveMu.Lock()
	defer s.moveMu.Unlock()
	sessionID, err := s.db.GetActiveSessionID()
	if err != nil {
		return err
	}
	if sessionID == 0 {
		return nil
	}
	if err := s.db.ClearSessionItems(sessionID); err != nil {
		return err
	}
	s.removeSessionFiles(sessionID)
	s.emit.Emit("session:cleared")
	return nil
}

func (s *Service) GetSessionImage(id int64) (string, error) {
	item, err := s.db.GetSessionItem(id)
	if err != nil {
		return "", fmt.Errorf("read session item: %w", err)
	}
	if item == nil {
		return "", fmt.Errorf("session item not found")
	}
	data, err := os.ReadFile(filepath.Join(s.sessionDir(item.SessionID), item.FileName))
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (s *Service) GetSessionThumb(id int64) (string, error) {
	item, err := s.db.GetSessionItem(id)
	if err != nil {
		return "", fmt.Errorf("read session item: %w", err)
	}
	if item == nil {
		return "", fmt.Errorf("session item not found")
	}
	data, err := os.ReadFile(filepath.Join(s.thumbDir(item.SessionID), item.ThumbName))
	if err != nil {
		return "", fmt.Errorf("read thumbnail: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
