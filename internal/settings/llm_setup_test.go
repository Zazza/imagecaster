package settings

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type llmStubModel struct {
	name    string
	caps    []string
	showErr bool
}

func llmSetupService(t *testing.T, models []llmStubModel, listFail bool, saved map[string]string) *Service {
	t.Helper()
	byName := map[string]llmStubModel{}
	for _, m := range models {
		byName[m.name] = m
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/server/models/llm":
			if listFail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("boom"))
				return
			}
			list := make([]serverclient.LLMModelInfo, 0, len(models))
			for _, m := range models {
				list = append(list, serverclient.LLMModelInfo{Name: m.name, Size: "5.0 GB"})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(list)
		case r.Method == http.MethodPost && r.URL.Path == "/api/llm/api/show":
			raw, _ := io.ReadAll(r.Body)
			var req map[string]any
			_ = json.Unmarshal(raw, &req)
			name, _ := req["model"].(string)
			m, ok := byName[name]
			if !ok || m.showErr {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"show failed"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": m.caps})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	s := map[string]string{"server_url": srv.URL, "connection_mode": "server"}
	for k, v := range saved {
		s[k] = v
	}
	db := restoreDB(t, s)
	client := serverclient.NewClient()
	client.SetBaseURL(srv.URL)
	svc, _ := testServiceWithDB(t, db, client)
	return svc
}

func llmCatalog(t *testing.T) *serverclient.Catalog {
	t.Helper()
	c, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	require.NotEmpty(t, c.LLMGenerate)
	require.NotEmpty(t, c.LLMVision)
	return c
}

func recommendedOf(t *testing.T, list []serverclient.CatalogModel) serverclient.CatalogModel {
	t.Helper()
	for _, m := range list {
		if m.Recommended {
			return m
		}
	}
	t.Fatal("catalog section has no recommended model")
	return serverclient.CatalogModel{}
}

func catalogNamesExcept(list []serverclient.CatalogModel, installed ...string) []string {
	skip := map[string]bool{}
	for _, n := range installed {
		skip[n] = true
	}
	out := []string{}
	for _, m := range list {
		if !skip[m.Name] {
			out = append(out, m.Name)
		}
	}
	return out
}

func choiceNames(list []LLMChoice, installed bool) []string {
	out := []string{}
	for _, c := range list {
		if c.Installed == installed {
			out = append(out, c.Name)
		}
	}
	return out
}

func findChoice(list []LLMChoice, name string) *LLMChoice {
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

func assertInstalledFirst(t *testing.T, list []LLMChoice) {
	t.Helper()
	seenCatalog := false
	for _, c := range list {
		if !c.Installed {
			seenCatalog = true
			continue
		}
		assert.False(t, seenCatalog, "installed model %q listed after models to download", c.Name)
	}
}

func assertNoDuplicates(t *testing.T, list []LLMChoice) {
	t.Helper()
	seen := map[string]bool{}
	for _, c := range list {
		assert.False(t, seen[c.Name], "model %q listed twice", c.Name)
		seen[c.Name] = true
	}
}

func TestGetLLMSetup_Needed(t *testing.T) {
	t.Parallel()
	server := []llmStubModel{
		{name: "text-a:4b", caps: []string{"completion"}},
		{name: "vis-a:8b", caps: []string{"completion", "vision"}},
	}
	tests := []struct {
		name     string
		saved    map[string]string
		listFail bool
		want     bool
	}{
		{name: "generate empty", saved: map[string]string{}, want: true},
		{name: "generate default", saved: map[string]string{"llm_generate_model": "default"}, want: true},
		{name: "generate on server analyze empty", saved: map[string]string{"llm_generate_model": "text-a:4b"}, want: false},
		{name: "generate and analyze on server", saved: map[string]string{"llm_generate_model": "text-a:4b", "llm_analyze_model": "vis-a:8b"}, want: false},
		{name: "analyze set and missing", saved: map[string]string{"llm_generate_model": "text-a:4b", "llm_analyze_model": "gone-vl:8b"}, want: true},
		{name: "generate missing", saved: map[string]string{"llm_generate_model": "gone:8b"}, want: true},
		{name: "server list fails", saved: map[string]string{}, listFail: true, want: false},
		{name: "server list fails with missing generate", saved: map[string]string{"llm_generate_model": "gone:8b"}, listFail: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := llmSetupService(t, server, tt.listFail, tt.saved)

			got, err := svc.GetLLMSetup()

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.Needed)
		})
	}
}

func TestGetLLMSetup_Lists(t *testing.T) {
	t.Parallel()
	cat := llmCatalog(t)
	recGen := recommendedOf(t, cat.LLMGenerate).Name
	recVis := recommendedOf(t, cat.LLMVision).Name
	server := []llmStubModel{
		{name: "bge-m3", caps: []string{"embedding"}},
		{name: recVis, caps: []string{"completion", "vision"}},
		{name: recGen, caps: []string{"completion"}},
		{name: "x", showErr: true},
	}
	tests := []struct {
		name          string
		list          func(*LLMSetup) []LLMChoice
		catalog       []serverclient.CatalogModel
		wantInstalled []string
	}{
		{name: "generate", list: func(s *LLMSetup) []LLMChoice { return s.Generate }, catalog: cat.LLMGenerate, wantInstalled: []string{recVis, recGen, "x"}},
		{name: "vision", list: func(s *LLMSetup) []LLMChoice { return s.Vision }, catalog: cat.LLMVision, wantInstalled: []string{recVis}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := llmSetupService(t, server, false, nil)

			setup, err := svc.GetLLMSetup()
			require.NoError(t, err)
			require.NotNil(t, setup)
			list := tt.list(setup)

			assert.ElementsMatch(t, tt.wantInstalled, choiceNames(list, true))
			assert.ElementsMatch(t, catalogNamesExcept(tt.catalog, tt.wantInstalled...), choiceNames(list, false))
			assert.Nil(t, findChoice(list, "bge-m3"))
			assertInstalledFirst(t, list)
			assertNoDuplicates(t, list)
			for _, m := range tt.catalog {
				c := findChoice(list, m.Name)
				if c == nil {
					continue
				}
				assert.Equal(t, m.Recommended, c.Recommended, m.Name)
				if !c.Installed {
					assert.InDelta(t, m.SizeGB, c.SizeGB, 0.001, m.Name)
				}
			}
		})
	}
}

func TestGetLLMSetup_Preselect(t *testing.T) {
	t.Parallel()
	cat := llmCatalog(t)
	recGen := recommendedOf(t, cat.LLMGenerate).Name
	recVis := recommendedOf(t, cat.LLMVision).Name
	text := []string{"completion"}
	vision := []string{"completion", "vision"}
	tests := []struct {
		name          string
		server        []llmStubModel
		saved         map[string]string
		wantGenerate  string
		wantVision    string
		wantGenRecNew bool
		wantVisRecNew bool
	}{
		{
			name:         "recommended installed",
			server:       []llmStubModel{{name: recGen, caps: text}, {name: recVis, caps: vision}, {name: "other-text:3b", caps: text}},
			wantGenerate: recGen,
			wantVision:   recVis,
		},
		{
			name:         "only other suitable installed",
			server:       []llmStubModel{{name: "other-text:3b", caps: text}, {name: "other-vl:7b", caps: vision}},
			wantGenerate: "",
			wantVision:   "",
		},
		{
			name:          "nothing suitable installed",
			server:        []llmStubModel{{name: "bge-m3", caps: []string{"embedding"}}},
			wantGenerate:  recGen,
			wantVision:    recVis,
			wantGenRecNew: true,
			wantVisRecNew: true,
		},
		{
			name:          "empty server",
			server:        []llmStubModel{},
			wantGenerate:  recGen,
			wantVision:    recVis,
			wantGenRecNew: true,
			wantVisRecNew: true,
		},
		{
			name:          "each list decides separately",
			server:        []llmStubModel{{name: "other-text:3b", caps: text}},
			wantGenerate:  "",
			wantVision:    recVis,
			wantVisRecNew: true,
		},
		{
			name:         "saved installed choice",
			server:       []llmStubModel{{name: recGen, caps: text}, {name: recVis, caps: vision}, {name: "other-text:3b", caps: text}, {name: "other-vl:7b", caps: vision}},
			saved:        map[string]string{"llm_generate_model": "other-text:3b", "llm_analyze_model": "other-vl:7b"},
			wantGenerate: "other-text:3b",
			wantVision:   "other-vl:7b",
		},
		{
			name:         "saved choice no longer installed",
			server:       []llmStubModel{{name: recGen, caps: text}, {name: recVis, caps: vision}},
			saved:        map[string]string{"llm_generate_model": "gone:8b", "llm_analyze_model": "gone-vl:8b"},
			wantGenerate: recGen,
			wantVision:   recVis,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := llmSetupService(t, tt.server, false, tt.saved)

			setup, err := svc.GetLLMSetup()
			require.NoError(t, err)
			require.NotNil(t, setup)

			assert.Equal(t, tt.wantGenerate, setup.GenerateSelected)
			assert.Equal(t, tt.wantVision, setup.VisionSelected)
			if tt.wantGenerate != "" {
				c := findChoice(setup.Generate, tt.wantGenerate)
				require.NotNil(t, c, "selected generate model must be in the list")
				assert.Equal(t, !tt.wantGenRecNew, c.Installed)
			}
			if tt.wantVision != "" {
				c := findChoice(setup.Vision, tt.wantVision)
				require.NotNil(t, c, "selected vision model must be in the list")
				assert.Equal(t, !tt.wantVisRecNew, c.Installed)
			}
		})
	}
}

func TestSaveLLMChoice(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		generate string
		vision   string
		wantErr  bool
	}{
		{name: "both set", generate: "qwen-text:8b", vision: "qwen-vl:8b"},
		{name: "same model for both", generate: "qwen-both:9b", vision: "qwen-both:9b"},
		{name: "empty generate", generate: "", vision: "qwen-vl:8b", wantErr: true},
		{name: "empty vision", generate: "qwen-text:8b", vision: "", wantErr: true},
		{name: "both empty", generate: "", vision: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db := testService(t, nil, nil)
			require.NoError(t, db.SetSetting("llm_generate_model", "prev-text"))
			require.NoError(t, db.SetSetting("llm_analyze_model", "prev-vl"))

			err := svc.SaveLLMChoice(tt.generate, tt.vision)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, "prev-text", setting(t, db, "llm_generate_model"))
				assert.Equal(t, "prev-vl", setting(t, db, "llm_analyze_model"))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.generate, setting(t, db, "llm_generate_model"))
			assert.Equal(t, tt.vision, setting(t, db, "llm_analyze_model"))
		})
	}
}
