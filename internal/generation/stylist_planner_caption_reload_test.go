package generation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	captionReloadDeadline = 3 * time.Second
	captionReloadServer   = "http://127.0.0.1:8234"
	captionReloadForeign  = "another-users-text-model"
	captionRetryMinimum   = time.Second
)

type captionReloadLLM struct {
	plannerModelLLM
	loaded             []llm.LoadedModel
	unloaded           []string
	unloadTimes        []time.Time
	deadlines          []time.Time
	otherRequestActive bool
	otherRequestDone   bool
	disappearOnFirst   bool
	permanentlyBusy    bool
	retryErr           error
	unloadAfterGone    bool
	unloadAllCalls     int
}

func (c *captionReloadLLM) IdleModelServerURL() string { return captionReloadServer }

func (c *captionReloadLLM) ListLoaded() ([]llm.LoadedModel, error) {
	if len(c.unloaded) > 0 && c.otherRequestActive && !c.permanentlyBusy {
		c.otherRequestActive = false
		c.otherRequestDone = true
	}
	return c.loaded, nil
}

func (c *captionReloadLLM) UnloadModel(ctx context.Context, model string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("caption release requires a deadline")
	}
	c.deadlines = append(c.deadlines, deadline)
	c.unloaded = append(c.unloaded, model)
	c.unloadTimes = append(c.unloadTimes, time.Now())
	if len(c.unloaded) == 1 && !c.disappearOnFirst {
		c.otherRequestActive = true
		return nil
	}
	if c.retryErr != nil {
		return c.retryErr
	}
	if c.otherRequestActive {
		return nil
	}
	found := false
	retained := make([]llm.LoadedModel, 0, len(c.loaded))
	for _, candidate := range c.loaded {
		if candidate.Name == model {
			found = true
			continue
		}
		retained = append(retained, candidate)
	}
	c.unloadAfterGone = !found
	c.loaded = retained
	return nil
}

func (c *captionReloadLLM) UnloadAll(context.Context) error {
	c.unloadAllCalls++
	return errors.New("must preserve unrelated loaded models")
}

func newCaptionReloadService(t *testing.T, client *captionReloadLLM) *Service {
	t.Helper()
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("llm_generate_model", plannerCaptionModel))
	client.loaded = []llm.LoadedModel{{Name: plannerCaptionModel}, {Name: captionReloadForeign}}
	return New(db, client, &plannerCapacitySD{}, &config.Config{SDPromptModel: plannerCaptionModel}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
}

func assertCaptionReloadTargetAndDeadline(t *testing.T, client *captionReloadLLM, deadline time.Time) {
	t.Helper()
	for _, model := range client.unloaded {
		assert.Equal(t, plannerCaptionModel, model)
	}
	for _, actual := range client.deadlines {
		assert.Equal(t, deadline, actual, "retries must retain the original deadline")
	}
	assert.Zero(t, client.unloadAllCalls)
	assert.False(t, client.unloadAfterGone)
	assert.Contains(t, client.loaded, llm.LoadedModel{Name: captionReloadForeign})
}

func TestPlannerCaptionReloadIsReleasedAfterOtherRequestCompletes(t *testing.T) {
	client := &captionReloadLLM{}
	svc := newCaptionReloadService(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), captionReloadDeadline)
	defer cancel()
	deadline, _ := ctx.Deadline()

	err := svc.releasePlannerCaption(ctx, captionReloadServer)

	require.NoError(t, err, "acknowledged unload raced with another request; its completed caption must be released again")
	assert.True(t, client.otherRequestDone)
	assert.GreaterOrEqual(t, len(client.unloaded), 2)
	assert.NotContains(t, client.loaded, llm.LoadedModel{Name: plannerCaptionModel})
	assertCaptionReloadTargetAndDeadline(t, client, deadline)
}

func TestPlannerCaptionReloadDoesNotRepeatAfterDisappearance(t *testing.T) {
	client := &captionReloadLLM{disappearOnFirst: true}
	svc := newCaptionReloadService(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), captionReloadDeadline)
	defer cancel()
	deadline, _ := ctx.Deadline()

	require.NoError(t, svc.releasePlannerCaption(ctx, captionReloadServer))
	assert.Equal(t, []string{plannerCaptionModel}, client.unloaded)
	assertCaptionReloadTargetAndDeadline(t, client, deadline)
}

func TestPlannerCaptionReloadRemainsBoundedWhenPermanentlyBusy(t *testing.T) {
	const schedulingAllowance = time.Second
	client := &captionReloadLLM{permanentlyBusy: true}
	svc := newCaptionReloadService(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), captionReloadDeadline)
	defer cancel()
	deadline, _ := ctx.Deadline()
	started := time.Now()

	err := svc.releasePlannerCaption(ctx, captionReloadServer)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), captionReloadDeadline+schedulingAllowance)
	assert.False(t, client.otherRequestDone)
	assert.Contains(t, client.loaded, llm.LoadedModel{Name: plannerCaptionModel})
	assert.GreaterOrEqual(t, len(client.unloadTimes), 2, "a busy caption must get another targeted release attempt")
	for i := 1; i < len(client.unloadTimes); i++ {
		assert.GreaterOrEqual(t, client.unloadTimes[i].Sub(client.unloadTimes[i-1]), captionRetryMinimum, "retries must not send unload requests faster than once a second")
	}
	assertCaptionReloadTargetAndDeadline(t, client, deadline)
}

func TestPlannerCaptionReloadPropagatesRepeatedUnloadFailure(t *testing.T) {
	retryErr := errors.New("second targeted unload failed")
	client := &captionReloadLLM{retryErr: retryErr}
	svc := newCaptionReloadService(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), captionReloadDeadline)
	defer cancel()
	deadline, _ := ctx.Deadline()

	err := svc.releasePlannerCaption(ctx, captionReloadServer)

	require.ErrorIs(t, err, retryErr)
	assert.True(t, client.otherRequestDone)
	assertCaptionReloadTargetAndDeadline(t, client, deadline)
}
