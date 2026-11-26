package generation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sourcePlanTestMaxInstructions = 8
const sourcePlanTestMaxRunes = 2000

func sourcePlanReply(t *testing.T, instructions ...string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"instructions": instructions})
	require.NoError(t, err)
	return string(b)
}

func TestPlanSourcePosesUsesExactOriginalAnalyzeModelAndWish(t *testing.T) {
	for _, locale := range []string{"ru", "en"} {
		t.Run(locale, func(t *testing.T) {
			db := openTestDB(t)
			require.NoError(t, db.SetSetting("llm_analyze_model", "source-pose-vision"))
			image := "data:image/png;base64," + makePNGBase64(t, 32, 32)
			const wish = "Raise the existing right hand"
			want := []string{"Move the existing right hand into the hair.", "Rest the existing right hand on the waist."}
			calls := 0
			svc := newTestService(t, db, &mockLLM{chatVisionFn: func(model, system, user, source string, _ float64, _ int) (string, error) {
				calls++
				assert.Equal(t, "source-pose-vision", model)
				assert.Equal(t, image, source, "vision must see the exact original, including a data URL prefix")
				assert.Contains(t, user, wish)
				assert.Contains(t, user, strconv.Itoa(len(want)))
				constraints := strings.ToLower(system)
				for _, concept := range []string{"identity", "background", "light"} {
					assert.Contains(t, constraints, concept)
				}
				assert.Regexp(t, `cloth|outfit|garment`, constraints)
				assert.Regexp(t, `existing|visible`, constraints)
				assert.Regexp(t, `limb|arm|hand`, constraints)
				assert.Regexp(t, `invent|hallucinat|unseen|never add|do not add`, constraints)
				assert.Regexp(t, map[string]string{"ru": `(?i)russian|русск|"ru"`, "en": `(?i)english|английск|"en"`}[locale], system+"\n"+user)
				return sourcePlanReply(t, want...), nil
			}}, &mockSD{})
			got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: image, Wish: wish, Count: len(want), Locale: locale})
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestPlanSourcePosesValidatesBeforeVision(t *testing.T) {
	valid := makePNGBase64(t, 32, 32)
	decoded, err := base64.StdEncoding.DecodeString(valid)
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		mutate func(*SourcePosePlanParams)
	}{
		{"missing image", func(p *SourcePosePlanParams) { p.ImageBase64 = "" }},
		{"invalid base64", func(p *SourcePosePlanParams) { p.ImageBase64 = "not-an-image!" }},
		{"base64 text", func(p *SourcePosePlanParams) {
			p.ImageBase64 = base64.StdEncoding.EncodeToString([]byte("this is not an image"))
		}},
		{"truncated image", func(p *SourcePosePlanParams) {
			p.ImageBase64 = base64.StdEncoding.EncodeToString(decoded[:len(decoded)/2])
		}},
		{"oversize image", func(p *SourcePosePlanParams) { p.ImageBase64 = genOverLimitBase64() }},
		{"zero count", func(p *SourcePosePlanParams) { p.Count = 0 }},
		{"negative count", func(p *SourcePosePlanParams) { p.Count = -1 }},
		{"too many variants", func(p *SourcePosePlanParams) { p.Count = sourcePlanTestMaxInstructions + 1 }},
		{"too long wish", func(p *SourcePosePlanParams) { p.Wish = strings.Repeat("я", sourcePlanTestMaxRunes+1) }},
		{"unsupported locale", func(p *SourcePosePlanParams) { p.Locale = "de" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := SourcePosePlanParams{ImageBase64: valid, Wish: "Smile", Count: 1, Locale: "en"}
			tc.mutate(&p)
			calls := 0
			svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) {
				calls++
				return sourcePlanReply(t, "Smile"), nil
			}}, &mockSD{})
			got, err := svc.PlanSourcePoses(p)
			require.Error(t, err)
			assert.Empty(t, got)
			assert.Zero(t, calls)
		})
	}
}

func TestPlanSourcePosesAcceptsOptionalWishAndRuneBoundaries(t *testing.T) {
	for _, wish := range []string{"", strings.Repeat("я", sourcePlanTestMaxRunes)} {
		want := make([]string, sourcePlanTestMaxInstructions)
		for i := range want {
			want[i] = "Pose " + strconv.Itoa(i+1)
		}
		want[0] = strings.Repeat("я", sourcePlanTestMaxRunes)
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return sourcePlanReply(t, want...), nil }}, &mockSD{})
		got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: makePNGBase64(t, 32, 32), Wish: wish, Count: len(want), Locale: "ru"})
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestPlanSourcePosesRejectsInvalidVisionPlanWithoutFallback(t *testing.T) {
	visionErr := errors.New("vision unavailable")
	for _, tc := range []struct {
		name, reply string
		err         error
	}{
		{name: "vision error", err: visionErr},
		{name: "interrupted", err: ErrInterrupted},
		{name: "empty"},
		{name: "invalid JSON", reply: "Here are two poses: smile and turn"},
		{name: "wrong shape", reply: `["Smile","Turn"]`},
		{name: "missing instructions", reply: `{}`},
		{name: "too few", reply: sourcePlanReply(t, "Smile")},
		{name: "too many", reply: sourcePlanReply(t, "Smile", "Turn", "Look left")},
		{name: "blank", reply: sourcePlanReply(t, "Smile", " \n ")},
		{name: "duplicate", reply: sourcePlanReply(t, "Smile", "Smile")},
		{name: "duplicate after trimming", reply: sourcePlanReply(t, "Smile", "  Smile ")},
		{name: "instruction too long", reply: sourcePlanReply(t, "Smile", strings.Repeat("я", sourcePlanTestMaxRunes+1))},
		{name: "trailing garbage", reply: sourcePlanReply(t, "Smile", "Turn") + " another answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return tc.reply, tc.err }}, &mockSD{})
			got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: makePNGBase64(t, 32, 32), Count: 2, Locale: "en"})
			require.Error(t, err)
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
			}
			assert.Empty(t, got)
		})
	}
}

func TestPlanSourcePosesNeverSendsPhotoToRemoteVision(t *testing.T) {
	calls := 0
	svc := newTestServiceURL(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) {
		calls++
		return sourcePlanReply(t, "Smile"), nil
	}}, "https://vision.example.com")
	got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: makePNGBase64(t, 32, 32), Count: 1, Locale: "en"})
	require.ErrorIs(t, err, ErrVisionNotLocal)
	assert.Empty(t, got)
	assert.Zero(t, calls)
}

func TestPlanSourcePosesDiscardsReplyAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) {
		cancel()
		return sourcePlanReply(t, "Smile"), nil
	}}, &mockSD{})
	svc.ctx = ctx
	got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: makePNGBase64(t, 32, 32), Count: 1, Locale: "en"})
	require.ErrorIs(t, err, ErrInterrupted)
	assert.Empty(t, got)
}

func TestPlanSourcePosesCancelledRequestNeverCallsVision(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) {
		calls++
		return sourcePlanReply(t, "Smile"), nil
	}}, &mockSD{})
	svc.ctx = ctx
	got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: makePNGBase64(t, 32, 32), Count: 1, Locale: "en"})
	require.ErrorIs(t, err, ErrInterrupted)
	assert.Empty(t, got)
	assert.Zero(t, calls)
}

func TestPlanSourcePosesKidsPolicies(t *testing.T) {
	t.Run("blocked wish is rejected before vision", func(t *testing.T) {
		calls := 0
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) {
			calls++
			return sourcePlanReply(t, "Smile"), nil
		}}, &mockSD{})
		require.NoError(t, svc.kids.SetKidsMode(true, "1234"))
		got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: makePNGBase64(t, 32, 32), Wish: "make the person look like a zombie", Count: 1, Locale: "en"})
		require.Error(t, err)
		assert.Empty(t, got)
		assert.Zero(t, calls)
	})
	t.Run("each returned instruction is filtered", func(t *testing.T) {
		const reply = "a zombie with short hair"
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return sourcePlanReply(t, reply), nil }}, &mockSD{})
		require.NoError(t, svc.kids.SetKidsMode(true, "1234"))
		expected := strings.TrimSpace(svc.kids.FilterOutput(reply))
		require.NotEqual(t, reply, expected)
		got, err := svc.PlanSourcePoses(SourcePosePlanParams{ImageBase64: makePNGBase64(t, 32, 32), Count: 1, Locale: "en"})
		require.NoError(t, err)
		assert.Equal(t, []string{expected}, got)
	})
}

func TestPrepareSourcePoseKeepsApprovedFinalPoseAndExactPhoto(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("llm_analyze_model", "source-pose-vision"))
	photo := "data:image/png;base64," + makePNGBase64(t, 32, 32)
	const approved = "Обе руки свободно опущены вдоль тела."
	const final = "Both existing arms hang relaxed alongside the body."
	calls := 0
	svc := newTestService(t, db, &mockLLM{chatVisionFn: func(model, system, user, image string, _ float64, _ int) (string, error) {
		calls++
		assert.Equal(t, "source-pose-vision", model)
		assert.Equal(t, photo, image)
		assert.Contains(t, user, approved)
		prompt := strings.ToLower(system)
		assert.Regexp(t, `final|requested|approved`, prompt)
		assert.Regexp(t, `translat|preserv|exact|faithful`, prompt)
		assert.Regexp(t, `(?s)(do not|never)[^.]*((unchanged|original|source)[^.]*limb|limb[^.]*(unchanged|original|source))`, prompt)
		return final, nil
	}}, &mockSD{})
	got, err := svc.PrepareSourcePose(photo, approved)
	require.NoError(t, err)
	assert.Equal(t, final, got)
	assert.Equal(t, 1, calls)
}
