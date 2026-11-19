package serverclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPersonDisplay_PostsJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lora     string
		display  string
		replace  bool
		respBody string
		wantPath string
		wantDisp string
	}{
		{
			name:     "set name",
			lora:     "natasha_curated",
			display:  "Наташа",
			replace:  false,
			respBody: `{"person":{"name":"natasha_curated","base":"Z-Image","lora":"natasha_curated","display_name":"Наташа"}}`,
			wantPath: "/api/train/people/natasha_curated/display",
			wantDisp: "Наташа",
		},
		{
			name:     "replace flag sent",
			lora:     "natasha_curated_moss",
			display:  "Наташа",
			replace:  true,
			respBody: `{"person":{"name":"natasha_curated_moss","base":"Moss","display_name":"Наташа"}}`,
			wantPath: "/api/train/people/natasha_curated_moss/display",
			wantDisp: "Наташа",
		},
		{
			name:     "empty clears",
			lora:     "anna",
			display:  "",
			replace:  false,
			respBody: `{"person":{"name":"anna","base":"Z-Image"}}`,
			wantPath: "/api/train/people/anna/display",
			wantDisp: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, http.StatusOK, tt.respBody)

			p, err := c.SetPersonDisplay(context.Background(), tt.lora, tt.display, tt.replace)

			require.NoError(t, err)
			req := rec.last(t)
			assert.Equal(t, http.MethodPost, req.method)
			assert.Equal(t, tt.wantPath, req.path)
			assert.Contains(t, req.ctype, "application/json")
			var body map[string]any
			require.NoError(t, json.Unmarshal(req.body, &body), string(req.body))
			assert.Equal(t, tt.display, body["display_name"])
			assert.Equal(t, tt.replace, body["replace"])
			require.NotNil(t, p)
			assert.Equal(t, tt.lora, p.Name)
			assert.Equal(t, tt.wantDisp, p.DisplayName)
		})
	}
}

func TestSetPersonDisplay_EscapesName(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"person":{"name":"a/b"}}`)

	_, err := c.SetPersonDisplay(context.Background(), "a/b", "Аня", false)

	require.NoError(t, err)
	assert.Equal(t, "/api/train/people/a%2Fb/display", rec.last(t).path)
}

func TestSetPersonDisplay_APIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		code   string
	}{
		{"taken", http.StatusConflict, "display_taken"},
		{"invalid", http.StatusBadRequest, "invalid_request"},
		{"not found", http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, tt.status, `{"error":{"code":"`+tt.code+`","message":"natasha_old"}}`)

			p, err := c.SetPersonDisplay(context.Background(), "natasha_curated", "Наташа", false)

			assert.Nil(t, p)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, tt.status, apiErr.Status)
			assert.Equal(t, tt.code, apiErr.Code)
			assert.Equal(t, "natasha_old", apiErr.Message)
		})
	}
}

func TestSubmitTraining_DisplayName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		display  string
		wantSent bool
	}{
		{"non-empty sent", "Наташа", true},
		{"empty not sent", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, http.StatusAccepted, trHoldJobJSON)
			paths, _ := trPhotos(t, 10)

			job, err := c.SubmitTraining(context.Background(), TrainRequest{
				Name:        "anna",
				Gender:      "woman",
				Client:      "inst-1",
				ClientRef:   "inst-1:train:anna",
				Photos:      paths,
				DisplayName: tt.display,
			})

			require.NoError(t, err)
			require.NotNil(t, job)
			mp := trParseMultipart(t, rec.last(t))
			got, ok := mp.fields["display_name"]
			if tt.wantSent {
				require.True(t, ok, "display_name field missing")
				assert.Equal(t, []string{tt.display}, got)
			} else {
				assert.False(t, ok, "display_name must be absent, got %v", got)
			}
			assert.Equal(t, []string{"anna"}, mp.fields["name"])
			assert.Len(t, mp.order, 10)
		})
	}
}

func TestPeople_DecodesDisplayName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"with names", `{"people":[{"name":"natasha_curated","display_name":"Наташа"},{"name":"anna"}]}`, []string{"Наташа", ""}},
		{"old server without field", `{"people":[{"name":"anna","base":"Z-Image"}]}`, []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, http.StatusOK, tt.body)

			list, err := c.People(context.Background())

			require.NoError(t, err)
			require.Len(t, list, len(tt.want))
			for i, w := range tt.want {
				assert.Equal(t, w, list[i].DisplayName)
			}
		})
	}
}

func TestPerson_DisplayNameOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(Person{Name: "anna"})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "display_name")

	b, err = json.Marshal(Person{Name: "anna", DisplayName: "Аня"})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"display_name":"Аня"`)
}
