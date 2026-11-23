package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/maskcomp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	objectOpRemove         = "remove"
	objectOpReplace        = "replace"
	objectMaxReplacement   = 500
	objectMaxObject        = 200
	objectSteps            = 4
	objectDenoise          = 1.0
	objectVisionMaxDim     = 1024
	objectVisionMaxTokens  = 40
	objectVisionTemp       = 0.1
	objectLabelMax         = 60
	objectMaxDim           = 2 * kleinLookMaxDim
	objectVisionSystem     = "You look at a photo where a red outline roughly marks an area. The marking is sloppy: it may cover extra background or miss parts. Identify the single main object the user marked. Answer with a short English noun phrase with color and position (e.g. 'the red coffee mug on the table'). No sentences, no explanation."
	objectVisionUser       = "What object is inside the red outline?"
	objectRemoveTemplate   = "Remove %s completely. Fill the area with the natural background that would be behind it. Keep everything else in the image unchanged."
	objectReplaceTemplate  = "Replace %s with %s. Keep everything else in the image unchanged."
	objectUndetectedErrMsg = "не удалось определить объект — впишите его вручную"
)

type ObjectEditParams struct {
	Editor      *PostEditorParams `json:"editor,omitempty"`
	ImageBase64 string            `json:"image_base64"`
	MaskBase64  string            `json:"mask_base64"`
	Op          string            `json:"op"`
	Replacement string            `json:"replacement"`
	Object      string            `json:"object"`
	Seed        *int64            `json:"seed"`
}

type objectInput struct {
	img         image.Image
	mask        image.Image
	op          string
	replacement string
	object      string
}

func (s *Service) validateObjectParams(p ObjectEditParams) (*objectInput, error) {
	if p.ImageBase64 == "" {
		return nil, errors.New("image is required")
	}
	if len(p.ImageBase64) > MaxImageBase64Len {
		return nil, config.ImageTooLarge("image")
	}
	if p.MaskBase64 == "" {
		return nil, errors.New("mask is required")
	}
	if len(p.MaskBase64) > MaxImageBase64Len {
		return nil, config.ImageTooLarge("mask")
	}
	if p.Op != objectOpRemove && p.Op != objectOpReplace {
		return nil, errors.New("op must be remove or replace")
	}
	replacement := strings.TrimSpace(sanitizeObjectText(p.Replacement))
	if p.Op == objectOpReplace {
		if replacement == "" {
			return nil, errors.New("replacement is required")
		}
		if len([]rune(replacement)) > objectMaxReplacement {
			return nil, fmt.Errorf("replacement too long (max %d characters)", objectMaxReplacement)
		}
	} else {
		replacement = ""
	}
	object := strings.TrimSpace(sanitizeObjectText(p.Object))
	if len([]rune(object)) > objectMaxObject {
		return nil, fmt.Errorf("object too long (max %d characters)", objectMaxObject)
	}
	var err error
	if replacement != "" {
		if replacement, err = s.kids.FilterInput(replacement); err != nil {
			return nil, err
		}
	}
	if object != "" {
		if object, err = s.kids.FilterInput(object); err != nil {
			return nil, err
		}
	}
	img, err := imgcodec.Decode(p.ImageBase64, imgcodec.Options{Name: "image", MaxDim: objectMaxDim})
	if err != nil {
		return nil, err
	}
	mask, err := imgcodec.Decode(p.MaskBase64, imgcodec.Options{Name: "mask", MaxDim: objectMaxDim})
	if err != nil {
		return nil, err
	}
	if !maskcomp.HasPaint(mask) {
		return nil, errors.New("mask is empty — paint over the object")
	}
	return &objectInput{img: img, mask: mask, op: p.Op, replacement: replacement, object: object}, nil
}

func (s *Service) detectMaskedObject(img, mask image.Image) (string, error) {
	if err := s.visionLocal(); err != nil {
		return "", err
	}
	overlay := image.Image(maskcomp.Outline(img, mask))
	b := overlay.Bounds()
	if w, h := b.Dx(), b.Dy(); w > objectVisionMaxDim || h > objectVisionMaxDim {
		ratio := math.Min(float64(objectVisionMaxDim)/float64(w), float64(objectVisionMaxDim)/float64(h))
		scaled := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*ratio)), max(1, int(float64(h)*ratio))))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), overlay, b, xdraw.Src, nil)
		overlay = scaled
	}
	overlayB64, err := imgcodec.EncodePNG(overlay, png.DefaultCompression)
	if err != nil {
		return "", fmt.Errorf("encode overlay: %w", err)
	}
	model := s.getAnalyzeModel()
	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	answer, err := s.llm.ChatVision(model, objectVisionSystem, objectVisionUser, overlayB64, objectVisionTemp, objectVisionMaxTokens)
	s.emitter.Emit("llm:status", map[string]string{"status": "done"})
	if err != nil {
		return "", fmt.Errorf("%s: %w", objectUndetectedErrMsg, err)
	}
	object := s.kids.FilterOutput(cleanObjectPhrase(answer))
	if object == "" {
		return "", errors.New(objectUndetectedErrMsg)
	}
	return object, nil
}

func cleanObjectPhrase(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.Trim(s, " \t\"'`.*")
	s = sanitizeObjectText(s)
	return truncateRunes(strings.TrimSpace(s), objectMaxObject)
}

func sanitizeObjectText(s string) string {
	return loraTagPrefixRe.ReplaceAllString(stripAngleTags(s), "")
}

func buildObjectInstruction(op, object, replacement string) string {
	if op == objectOpReplace {
		return fmt.Sprintf(objectReplaceTemplate, object, replacement)
	}
	return fmt.Sprintf(objectRemoveTemplate, object)
}

func (s *Service) EditObject(params ObjectEditParams) (*GenerateImageResult, error) {
	prof, err := resolvePostEditor(params.Editor, objectSteps)
	if err != nil {
		return nil, err
	}
	in, err := s.validateObjectParams(params)
	if err != nil {
		return nil, err
	}

	object := in.object
	if object == "" {
		s.emitter.Emit("object:stage", "analyzing")
		if object, err = s.detectMaskedObject(in.img, in.mask); err != nil {
			return nil, err
		}
	}
	instruction := buildObjectInstruction(in.op, object, in.replacement)

	b := in.img.Bounds()
	width, height := alignLookSize(b.Dx(), b.Dy())
	resp, err := s.postReferenceImg2Img(prof, "object", func() { s.emitter.Emit("object:stage", "generating") }, sd.Img2ImgRequest{
		InitImages:     []string{params.ImageBase64},
		Prompt:         instruction,
		NegativePrompt: s.kids.ApplyNegative(""),
		Width:          width,
		Height:         height,
		Seed:           params.Seed,
	})
	if err != nil {
		return nil, err
	}
	edited, err := imgcodec.Decode(resp.Images[0], imgcodec.Options{Name: "result", MaxDim: objectMaxDim})
	if err != nil {
		return nil, err
	}
	finalB64, err := imgcodec.EncodePNG(maskcomp.Composite(in.img, edited, in.mask), png.DefaultCompression)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}

	label := "Удаление: " + truncateRunes(object, objectLabelMax)
	if in.op == objectOpReplace {
		label = "Замена: " + truncateRunes(in.replacement, objectLabelMax)
	}
	info := patchInfoSize(resp.Info, b.Dx(), b.Dy())
	result := &GenerateImageResult{
		Image:           finalB64,
		Info:            info,
		EffectivePrompt: prof.prompt(instruction),
		DetectedObject:  object,
	}
	result.SessionItemID = s.sessions.AddToSession(finalB64, info, "object", &preset.GenerationMeta{
		Label: label,
		Model: prof.model,
	})
	return result, nil
}

func patchInfoSize(info json.RawMessage, w, h int) json.RawMessage {
	if len(info) == 0 {
		return info
	}
	raw := []byte(info)
	var inner string
	wrapped := json.Unmarshal(info, &inner) == nil
	if wrapped {
		raw = []byte(inner)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return info
	}
	m["width"], m["height"] = w, h
	out, err := json.Marshal(m)
	if err != nil {
		return info
	}
	if wrapped {
		if out, err = json.Marshal(string(out)); err != nil {
			return info
		}
	}
	return out
}
