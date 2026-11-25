package stylist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhotographyRecoveryCarriesActualSeriesReferenceSeparatelyFromMixedLocks(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "auto"
		if mixed {
			name = "mixed"
		}
		t.Run(name, func(t *testing.T) {
			initial, missing := seriesRecoveryReplies(t)
			var initialPlan map[string]any
			require.NoError(t, json.Unmarshal([]byte(initial), &initialPlan))
			first := autoTestSeries(initialPlan)
			first["background"] = "a cafe terrace with detailed stone architecture"
			first["place"] = "urban"
			first["background_ru"] = "Терраса кафе"
			first["outfit_ru"] = "Перевод одежды"
			req := autoTestRequest()
			req.Plan.Frames = seriesRecoveryFrames
			req.Target = autoTestTarget
			const explicitHair = "neat braided bun"
			if mixed {
				req.Plan.Aspects = map[string]AspectChoice{"hair": {Mode: ModeCustom, Text: explicitHair}}
			}
			chat := &scriptedChat{replies: []string{recoveryJSON(t, initialPlan), recoveryJSON(t, missing)}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.Len(t, got.Series, len(SeriesSizes(seriesRecoveryFrames)))
			require.Len(t, chat.calls, 2)
			assertContextBudget(t, chat.calls[1])
			var input map[string]any
			require.NoError(t, json.Unmarshal([]byte(chat.calls[1].user), &input))
			reference, ok := input["reference_series"].(map[string]any)
			require.True(t, ok, "recovery needs the actual first series as a compact reference")
			for _, field := range []string{"outfit", "background", "place"} {
				assert.Equal(t, first[field], reference[field], "reference %s must come from actual main response", field)
			}
			assert.NotContains(t, reference, "shots", "reference carries visual continuity, not already generated shots")
			for key := range reference {
				assert.NotContains(t, key, "_ru", "translations do not belong in the generation reference")
			}
			if mixed {
				t.Logf("recovery context: %s", chat.calls[1].user)
				constraints, ok := input["constraints"].([]any)
				require.True(t, ok, "the missing series must keep its own constraints")
				require.Len(t, constraints, 1)
				constraint, ok := constraints[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, explicitHair, constraint["hair"], "mixed locks must remain distinct from reference_series")
				assert.Equal(t, explicitHair, got.Series[1].Hair)
			}
		})
	}
}
