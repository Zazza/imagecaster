package generation

import (
	"errors"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const plannerCaptionModel = "hf.co/GitMylo/nsfwcaption-qwen3-vl-8b-v3-gguf:Q4_K_M"
const plannerText14B = "huihui_ai/qwen2.5-abliterate:14b"

type plannerModelLLM struct {
	mockLLM
	installed []llm.LLMModel
	listErr   error
	lists     int
}

func (m *plannerModelLLM) GetModels() ([]llm.LLMModel, error) {
	m.lists++
	return m.installed, m.listErr
}

type plannerCapacitySD struct{ mockSD }

func (*plannerCapacitySD) MemoryInfo() (*sd.MemoryStats, error) {
	const healthyFreeBytes = 32 * 1024 * 1024 * 1024
	return &sd.MemoryStats{RAM: sd.MemoryPool{Free: healthyFreeBytes}, CUDA: &sd.CUDAMemory{System: sd.MemoryPool{Free: healthyFreeBytes}}}, nil
}

func TestStylistPlannerModelPreservesOrdinaryConfiguredModelWithoutListing(t *testing.T) {
	db := openTestDB(t)
	const ordinary = "my-custom-text-planner"
	require.NoError(t, db.SetSetting("llm_generate_model", ordinary))
	client := &plannerModelLLM{listErr: errors.New("listing must not occur")}
	svc := New(db, client, &plannerCapacitySD{}, &config.Config{SDPromptModel: "config-default"}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
	got, err := svc.StylistPlannerModel()
	require.NoError(t, err)
	assert.Equal(t, ordinary, got)
	assert.Zero(t, client.lists)
	assert.Equal(t, ordinary, svc.getGenerateModel())
}

func TestStylistPlannerModelReplacesCaptionOnlyWithInstalledCatalogText(t *testing.T) {
	catalog, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	var recommended, firstOther string
	for _, candidate := range catalog.LLMGenerate {
		if candidate.Recommended && recommended == "" {
			recommended = candidate.Name
		}
		if !candidate.Recommended && candidate.Name != "qwen3:4b" && candidate.Name != plannerText14B && firstOther == "" {
			firstOther = candidate.Name
		}
	}
	require.NotEmpty(t, recommended, "the text catalog must identify its recommended planner")
	require.NotEmpty(t, firstOther, "exercise fallback to an installed non-recommended catalog entry")
	cases := []struct {
		name      string
		installed []string
		want      string
		listErr   error
		wantErr   bool
	}{
		{name: "recommended beats installation order", installed: []string{firstOther, recommended, plannerCaptionModel}, want: recommended},
		{name: "nonrecommended catalog text", installed: []string{plannerCaptionModel, firstOther}, want: firstOther},
		{name: "verified 14b beats recommended and broken small", installed: []string{"qwen3:4b", recommended, plannerText14B}, want: plannerText14B},
		{name: "only broken small installed", installed: []string{"qwen3:4b"}, wantErr: true},
		{name: "confirmed 14b text model", installed: []string{plannerCaptionModel, plannerText14B}, want: plannerText14B},
		{name: "caption and embedding are insufficient", installed: []string{plannerCaptionModel, "nomic-embed-text:latest"}, wantErr: true},
		{name: "no installed model", wantErr: true},
		{name: "listing unavailable", listErr: errors.New("installed model list unavailable"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			require.NoError(t, db.SetSetting("llm_generate_model", plannerCaptionModel))
			client := &plannerModelLLM{listErr: tc.listErr}
			for _, name := range tc.installed {
				client.installed = append(client.installed, llm.LLMModel{ID: name})
			}
			svc := New(db, client, &plannerCapacitySD{}, &config.Config{SDPromptModel: plannerCaptionModel}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
			got, err := svc.StylistPlannerModel()
			if tc.wantErr {
				require.Error(t, err)
				assert.NotEmpty(t, err.Error())
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
			assert.Equal(t, 1, client.lists)
			assert.Equal(t, plannerCaptionModel, svc.getGenerateModel(), "resolving an automatic planner must not rewrite the configured prompt model")
		})
	}
}
