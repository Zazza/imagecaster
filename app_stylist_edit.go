package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

const (
	stylistEditInstructionMax = 2000
	stylistEditModel          = "flux-2-klein-4b"
	stylistEditPreserve       = generation.FrameEditPreserveSuffix
)

type StylistEditParams struct {
	validateSource    func(*preset.SessionItem) error
	RestoreMaskBase64 string                       `json:"restore_mask_base64,omitempty"`
	RestoreStage      string                       `json:"restore_stage,omitempty"`
	RestoreAligned    bool                         `json:"restore_aligned,omitempty"`
	JobID             int64                        `json:"job_id"`
	Instruction       string                       `json:"instruction"`
	Editor            *generation.PostEditorParams `json:"editor"`
}

type stylistReferenceEditEngine interface {
	StylistReferenceEdit(imageBase64, instruction string, seed *int64, editor *generation.PostEditorParams) (*generation.GenerateImageResult, error)
}

type stylistEditQueue interface {
	stylistQueue
	stylistFrameResults
}

type stylistEditItems interface {
	SessionItem(id int64) (*preset.SessionItem, error)
}

type stylistEditEngine interface {
	PrepareStylistEdit(imageBase64, instruction string) (string, error)
	KleinEdit(imageBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error)
	FaceSwap(imageBase64, person string) (string, string)
}

func stylistEditInstruction(instruction string) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", errors.New("напишите, что изменить в кадре")
	}
	if len([]rune(instruction)) > stylistEditInstructionMax {
		return "", fmt.Errorf("пожелание слишком длинное: максимум %d символов", stylistEditInstructionMax)
	}
	return instruction, nil
}

func enqueueStylistEdit(st stylistSettings, q stylistEditQueue, items stylistEditItems, notLocal bool, jobID int64, instruction string) (int64, error) {
	return enqueueStylistEditParams(st, q, items, notLocal, StylistEditParams{JobID: jobID, Instruction: instruction})
}

func enqueueStylistEditWithEditor(st stylistSettings, q stylistEditQueue, items stylistEditItems, notLocal bool, params StylistEditParams) (int64, error) {
	editor, err := generation.NormalizePostEditor(params.Editor)
	if err != nil {
		return 0, err
	}
	params.Editor = editor
	return enqueueStylistEditParams(st, q, items, notLocal, params)
}

func enqueueStylistEditParams(st stylistSettings, q stylistEditQueue, items stylistEditItems, notLocal bool, params StylistEditParams) (int64, error) {
	jobID, instruction := params.JobID, params.Instruction
	if notLocal {
		return 0, errStylistPageNotLocal
	}
	instruction, err := stylistEditInstruction(instruction)
	if err != nil {
		return 0, err
	}
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	frames := loadLastShoot(st)
	var sourceID int64
	for _, f := range frames {
		if f.JobID == jobID && f.Status == string(queue.StatusCompleted) {
			sourceID = f.ItemID
			break
		}
	}
	if sourceID <= 0 {
		return 0, errors.New("выберите готовый кадр текущей фотосессии")
	}
	if len(frames) >= stylistFramesMax {
		return 0, fmt.Errorf("в фотосессии уже %d кадров", stylistFramesMax)
	}
	jobs, err := q.Results([]int64{jobID})
	if err != nil {
		return 0, err
	}
	var result mcp.ShootFrameResult
	ready := true
	for _, job := range jobs {
		if job == nil || job.ID != jobID {
			continue
		}
		ready = false
		if job.Type != queue.JobStylistFrame || job.Status != queue.StatusCompleted {
			break
		}
		var raw queue.JobResult
		if json.Unmarshal([]byte(job.Result), &raw) == nil && json.Unmarshal(raw.Frame, &result) == nil && result.ID == sourceID {
			ready = true
		}
		break
	}
	if !ready {
		return 0, errors.New("исходный кадр не найден среди готовых заданий")
	}
	item, err := items.SessionItem(sourceID)
	if err != nil {
		return 0, err
	}
	if item == nil || item.ID != sourceID || item.SessionID <= 0 {
		return 0, errors.New("исходного кадра больше нет в истории")
	}
	if params.validateSource != nil {
		if err := params.validateSource(item); err != nil {
			return 0, err
		}
	}
	store, err := loadShootParams(st)
	if err != nil {
		return 0, err
	}
	original, ok := store.Frames[strconv.FormatInt(jobID, 10)]
	if !ok {
		return 0, errors.New("нет сохранённых параметров исходного кадра")
	}
	swapName := original.Params.FaceSwapName
	if swapName == "" && original.Params.Pipeline == 0 {
		swapName = original.Params.Person
	}
	model := stylistEditModel
	if params.Editor != nil {
		model = params.Editor.Model
	}
	p := mcp.ShootFrameParams{
		Editor:            params.Editor,
		RestoreMaskBase64: params.RestoreMaskBase64, RestoreStage: params.RestoreStage, RestoreAligned: params.RestoreAligned,
		SourceItemID: item.ID, SourceSessionID: item.SessionID, EditInstruction: instruction,
		Person: original.Params.Person, Base: original.Params.Base, Model: model, Prompt: original.Params.Prompt,
		FaceSwap: original.Params.FaceSwap, FaceSwapName: swapName,
		FacePass: original.Params.FacePass, FaceRestoreBlend: original.Params.FaceRestoreBlend,
		Shot: instruction, ToFolder: true,
	}
	label := clipRunes("Стилист: изменить кадр — "+instruction, stylistPageLabelMax)
	id, err := q.EnqueueLabeled(queue.JobStylistFrame, p, stylistPageSource, label)
	if err != nil {
		return 0, err
	}
	store.Frames[strconv.FormatInt(id, 10)] = shootParamsFrame{Label: label, Params: p}
	raw, err := json.Marshal(store)
	if err == nil {
		err = st.SetSetting(stylistShootParamsKey, string(raw))
	}
	if err != nil {
		return id, fmt.Errorf("вариант поставлен в очередь, но параметры не сохранены: %w", err)
	}
	frames = append(frames, StylistFrame{JobID: id, Status: string(queue.StatusPending), Shot: instruction})
	if err := saveLastShoot(st, frames); err != nil {
		return id, fmt.Errorf("вариант поставлен в очередь, но съёмка не сохранена: %w", err)
	}
	return id, nil
}

func runStylistEdit(eng stylistEditEngine, sessions mcp.AppSessionItems, p mcp.ShootFrameParams, stopped func() bool) (*mcp.ShootFrameResult, error) {
	if eng == nil || sessions == nil {
		return nil, errors.New("изменить кадр: генерация или история недоступна")
	}
	if p.SourceItemID <= 0 || p.SourceSessionID <= 0 {
		return nil, errors.New("исходный кадр или папка не указаны")
	}
	instruction, err := stylistEditInstruction(p.EditInstruction)
	if err != nil {
		return nil, err
	}
	model := stylistEditModel
	var referenceEngine stylistReferenceEditEngine
	if p.Editor != nil {
		editor, err := generation.NormalizePostEditor(p.Editor)
		if err != nil {
			return nil, err
		}
		p.Editor, model = editor, editor.Model
		var ok bool
		referenceEngine, ok = eng.(stylistReferenceEditEngine)
		if !ok {
			return nil, errors.New("выбранный редактор кадров недоступен")
		}
	}
	if stopped == nil {
		stopped = func() bool { return false }
	}
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	item, err := sessions.SessionItem(p.SourceItemID)
	if err != nil {
		return nil, err
	}
	if item == nil || item.ID != p.SourceItemID || item.SessionID != p.SourceSessionID || p.SourceSessionID <= 0 {
		return nil, errors.New("исходный кадр удалён или перемещён")
	}
	source, err := sessions.GetSessionImage(item.ID)
	if err != nil {
		return nil, err
	}
	if source == "" {
		return nil, errors.New("исходное изображение не прочитано")
	}
	prepare := eng.PrepareStylistEdit
	if p.SourcePose {
		strict, ok := eng.(interface {
			PrepareSourcePose(string, string) (string, error)
		})
		if !ok {
			return nil, errors.New("подготовка поз исходного фото недоступна")
		}
		prepare = strict.PrepareSourcePose
	}
	prepared, err := prepare(source, instruction)
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	if err != nil {
		return nil, fmt.Errorf("не удалось подготовить изменение кадра: %w", err)
	}
	prepared, err = stylistEditInstruction(prepared)
	if err != nil {
		return nil, fmt.Errorf("ИИ не подготовил изменение кадра: %w", err)
	}
	prompt := prepared + stylistEditPreserve
	var res *generation.GenerateImageResult
	if p.Editor == nil {
		res, err = eng.KleinEdit(source, prompt, p.Seed)
	} else {
		res, err = referenceEngine.StylistReferenceEdit(source, prompt, p.Seed, p.Editor)
	}
	if err != nil {
		return nil, err
	}
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	if res == nil || res.Image == "" {
		return nil, errors.New("правка не вернула изображение")
	}
	facePass := generation.FaceSwapOff
	if p.FacePass {
		if faceEngine, ok := eng.(interface {
			mcp.StylistFaceLandmarker
			mcp.FaceRefiner
		}); ok {
			var interrupted bool
			res.Image, facePass, interrupted = mcp.RefineStylistFace(faceEngine, faceEngine, res.Image, p.Prompt, p.Base, p.Seed)
			if interrupted || stopped() {
				return nil, generation.ErrInterrupted
			}
		} else {
			facePass = "skipped:второй проход недоступен"
		}
	}
	faceSwap := generation.FaceSwapOff
	if p.FaceSwap {
		if p.FaceSwapName == "" {
			return nil, errors.New("не сохранено лицо для подмены")
		}
		if p.FaceRestoreBlend != nil {
			if !generation.ValidFaceRestoreBlend(*p.FaceRestoreBlend) {
				faceSwap = generation.FaceSwapSkipped("invalid_restore_blend")
			} else if restorer, ok := eng.(mcp.StylistFaceRestoreSwapper); ok {
				res.Image, faceSwap = restorer.FaceSwapWithRestore(res.Image, p.FaceSwapName, *p.FaceRestoreBlend)
			} else {
				faceSwap = "skipped:настройка восстановления лица недоступна в этой сборке"
			}
		} else {
			res.Image, faceSwap = eng.FaceSwap(res.Image, p.FaceSwapName)
		}
	}
	if stopped() || faceSwap == generation.FaceSwapInterrupted {
		return nil, generation.ErrInterrupted
	}
	if res.Image == "" {
		return nil, errors.New("подмена лица не вернула изображение")
	}
	current, err := sessions.SessionItem(p.SourceItemID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ID != p.SourceItemID || current.SessionID != p.SourceSessionID {
		return nil, errors.New("исходный кадр удалён или перемещён")
	}
	meta := &preset.GenerationMeta{Model: model, Label: clipRunes("Стилист: "+instruction, stylistPageLabelMax), SourceItem: item.ID}
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	id := sessions.AddToSessionID(p.SourceSessionID, res.Image, res.Info, stylistPageSource, meta)
	if id <= 0 {
		return nil, errors.New("изменённый кадр не сохранён в историю")
	}
	return &mcp.ShootFrameResult{ID: id, Shot: instruction, Prompt: prompt, FacePass: facePass, FaceSwap: faceSwap}, nil
}

func (a *App) StylistEdit(jobID int64, instruction string) (int64, error) {
	if a.presets == nil || a.queueSvc == nil || a.sessions == nil {
		return 0, errors.New("Стилист: история или очередь недоступна")
	}
	return enqueueStylistEdit(a.presets, a.queueSvc, a.sessions, sdNotLocal(a.config), jobID, instruction)
}

func (a *App) StylistEditWithEditor(params StylistEditParams) (int64, error) {
	if a.presets == nil || a.queueSvc == nil || a.sessions == nil {
		return 0, errors.New("Стилист: история или очередь недоступна")
	}
	return enqueueStylistEditWithEditor(a.presets, a.queueSvc, a.sessions, sdNotLocal(a.config), params)
}
