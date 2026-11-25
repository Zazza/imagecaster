package stylist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhotographySelectorBoundedStructuralRepair(t *testing.T) {
	const invalid = `{"target":"moss_image_sft_bf16","reason":"Photographic light"}`
	const valid = `{"target":"preset:chosen","reason":"Photographic light"}`
	options := []ShootOption{{ID: "preset:chosen", Name: "Moss photograph"}, {ID: "preset:other", Name: "Other photograph"}}
	cases := []struct {
		name      string
		replies   []string
		failureAt int
		calls     int
		wantError bool
	}{
		{name: "filename then valid option ID", replies: []string{invalid, valid, autoTestReply(t, nil)}, calls: 3},
		{name: "repeated filename fails closed", replies: []string{invalid, invalid}, calls: 2, wantError: true},
		{name: "provider error is not structural retry", failureAt: 1, calls: 1, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Locale = ""
			chat := &scriptedChat{replies: tc.replies, failureAt: tc.failureAt}
			got, err := Suggest(chat, "planner", req, mustBase(t), options)
			if tc.wantError {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "preset:chosen", got.Target)
			}
			require.Len(t, chat.calls, tc.calls)
			selectorCalls := tc.calls
			if !tc.wantError {
				selectorCalls--
			}
			for _, call := range chat.calls[:selectorCalls] {
				assert.Contains(t, call.user, "preset:chosen")
				assert.Contains(t, call.user, "preset:other")
				assertContextBudget(t, call)
			}
		})
	}
}
