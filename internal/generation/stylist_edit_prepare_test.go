package generation

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const prepareTestMaxRunes = 2000
const prepareTestTokens = 220

func TestPrepareStylistEditUsesExactImageWishAndAnalyzeModel(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("llm_analyze_model", "pose-vision-model"))
	image := "data:image/png;base64," + makePNGBase64(t, 64, 64)
	for _, wish := range []string{"Hand in hair", "Smile broadly", "Open the jacket"} {
		t.Run(wish, func(t *testing.T) {
			calls := 0
			svc := newTestService(t, db, &mockLLM{chatVisionFn: func(model, system, user, source string, temp float64, tokens int) (string, error) {
				calls++
				assert.Equal(t, "pose-vision-model", model)
				assert.NotEmpty(t, system)
				assert.Contains(t, user, wish)
				assert.Equal(t, image, source)
				assert.Zero(t, temp)
				assert.Equal(t, prepareTestTokens, tokens)
				return "  The requested final state.  ", nil
			}}, &mockSD{})
			got, err := svc.PrepareStylistEdit(image, " "+wish+" ")
			require.NoError(t, err)
			assert.Equal(t, "The requested final state.", got)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestPrepareStylistEditValidationStopsBeforeVision(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, image, wish string }{
		{name: "missing image", wish: "Smile"},
		{name: "image limit", image: strings.Repeat("x", MaxImageBase64Len+1), wish: "Smile"},
		{name: "missing wish", image: "image"},
		{name: "blank wish", image: "image", wish: " \n "},
		{name: "wish limit", image: "image", wish: strings.Repeat("я", prepareTestMaxRunes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return "unexpected", nil }}, &mockSD{})
			got, err := svc.PrepareStylistEdit(tc.image, tc.wish)
			require.Error(t, err)
			assert.Empty(t, got)
			assert.Zero(t, calls)
		})
	}
}

func TestPrepareStylistEditNeverSendsImageToRemoteVision(t *testing.T) {
	t.Parallel()
	calls := 0
	svc := newTestServiceURL(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return "unexpected", nil }}, "https://vision.example.com")
	got, err := svc.PrepareStylistEdit(makePNGBase64(t, 64, 64), "Smile")
	require.ErrorIs(t, err, ErrVisionNotLocal)
	assert.Empty(t, got)
	assert.Zero(t, calls)
}

func TestPrepareStylistEditRejectsInvalidVisionReply(t *testing.T) {
	t.Parallel()
	visionErr := errors.New("vision failed")
	for _, tc := range []struct {
		name, reply string
		err         error
	}{
		{name: "failure", err: visionErr},
		{name: "empty"},
		{name: "blank", reply: " \n "},
		{name: "reply limit", reply: strings.Repeat("я", prepareTestMaxRunes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return tc.reply, tc.err }}, &mockSD{})
			got, err := svc.PrepareStylistEdit(makePNGBase64(t, 64, 64), "Smile")
			require.Error(t, err)
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
			}
			assert.Empty(t, got)
		})
	}
}

func TestPrepareStylistEditKidsInputAndOutput(t *testing.T) {
	t.Parallel()
	t.Run("blocked input never reaches vision", func(t *testing.T) {
		calls := 0
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return "unexpected", nil }}, &mockSD{})
		require.NoError(t, svc.kids.SetKidsMode(true, "1234"))
		_, err := svc.PrepareStylistEdit(makePNGBase64(t, 64, 64), "make the person look like a zombie")
		require.Error(t, err)
		assert.Zero(t, calls)
	})
	t.Run("vision output is filtered", func(t *testing.T) {
		const reply = "a zombie with short hair"
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return reply, nil }}, &mockSD{})
		require.NoError(t, svc.kids.SetKidsMode(true, "1234"))
		expected := strings.TrimSpace(svc.kids.FilterOutput(reply))
		require.NotEqual(t, reply, expected)
		got, err := svc.PrepareStylistEdit(makePNGBase64(t, 64, 64), "short hair")
		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})
}
