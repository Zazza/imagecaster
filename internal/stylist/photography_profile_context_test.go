package stylist

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhotographyFullSelectedProfileReachesMainAndRecovery(t *testing.T) {
	const selectedID = "preset:photographic-context"
	const tail = "Deep focus preserves terrace architecture; noir side lighting and crisp textile detail."
	const irrelevant = "UNSELECTED_PROFILE_DESCRIPTION_MUST_STAY_OUT"
	const prefixRepeats = 20
	description := strings.Repeat("Photographic continuity. ", prefixRepeats) + tail
	for _, mode := range []string{"main", "missing_series", "missing_shot"} {
		t.Run(mode, func(t *testing.T) {
			req := autoTestRequest()
			req.Target = selectedID
			options := []ShootOption{
				{ID: selectedID, Name: "Photographic profile", Model: "installed-model", Category: "stylist_photo", Description: description},
				{ID: "preset:unselected", Name: "Other style", Description: irrelevant},
			}
			before, err := json.Marshal(options)
			require.NoError(t, err)
			initial := autoTestReply(t, nil)
			var supplements []string
			if mode == "missing_series" {
				req.Plan.Frames = seriesRecoveryFrames
				var missing map[string]any
				initial, missing = seriesRecoveryReplies(t)
				supplements = append(supplements, recoveryJSON(t, missing))
			}
			var plan map[string]any
			require.NoError(t, json.Unmarshal([]byte(initial), &plan))
			plan["target"] = "preset:hallucinated"
			plan["target_name"] = "Hallucinated profile"
			plan["target_model"] = "Hallucinated model"
			if mode == "missing_shot" {
				autoTestSeries(plan)["shots"] = []any{}
				supplements = append(supplements, singleShotJSON(t, 0))
			}
			chat := &scriptedChat{replies: append([]string{recoveryJSON(t, plan)}, supplements...)}
			got, err := Suggest(chat, "planner", req, mustBase(t), options)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, selectedID, got.Target, "planner hallucination must not replace validated selection")
			assert.Equal(t, options[0].Name, got.TargetName)
			assert.Equal(t, options[0].Model, got.TargetModel)
			require.Len(t, chat.calls, len(supplements)+1)
			for index, call := range chat.calls {
				assert.Contains(t, call.user, description, "call %d needs the full original selected profile", index)
				assert.Contains(t, call.user, tail, "call %d must retain lighting/style/deep-focus instructions past the preview", index)
				assert.NotContains(t, call.user, irrelevant)
				assertContextBudget(t, call)
			}
			after, err := json.Marshal(options)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after), "candidate descriptions are caller-owned")
		})
	}
}
