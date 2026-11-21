package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStylistNamedPlan_SaveListRoundTrip(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)
	plan := `{"series":[{"shots":[{"pose":"стоя"}]}]}`
	saved, err := app.StylistSaveNamedPlan("Осень в парке", plan, false)
	require.NoError(t, err)
	require.NotNil(t, saved)

	list, err := app.StylistPlans()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Осень в парке", list[0].Name)
	assert.Equal(t, plan, list[0].Plan)
}

func TestStylistNamedPlan_EmptyList(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)
	list, err := app.StylistPlans()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStylistNamedPlan_InvalidName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"tabs and newlines", "\t\n "},
		{"81 cyrillic letters", strings.Repeat("я", 81)},
		{"81 latin letters", strings.Repeat("a", 81)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _ := newTestApp(t)
			_, err := app.StylistSaveNamedPlan(tt.in, `{"a":1}`, false)
			assert.Error(t, err)
			list, err := app.StylistPlans()
			require.NoError(t, err)
			assert.Empty(t, list)
		})
	}
}

func TestStylistNamedPlan_NameLimitCountsLetters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
	}{
		{"80 cyrillic letters", strings.Repeat("я", 80)},
		{"80 latin letters", strings.Repeat("a", 80)},
		{"80 letters plus outer spaces", "  " + strings.Repeat("ж", 80) + "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _ := newTestApp(t)
			_, err := app.StylistSaveNamedPlan(tt.in, `{"a":1}`, false)
			require.NoError(t, err)
			list, err := app.StylistPlans()
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, strings.TrimSpace(tt.in), list[0].Name)
		})
	}
}

func TestStylistNamedPlan_InvalidPlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		plan string
	}{
		{"empty", ""},
		{"not json", "not json"},
		{"array", "[1]"},
		{"null", "null"},
		{"string", `"plan"`},
		{"number", "42"},
		{"broken object", `{"a":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _ := newTestApp(t)
			_, err := app.StylistSaveNamedPlan("Осень", tt.plan, false)
			assert.Error(t, err)
			list, err := app.StylistPlans()
			require.NoError(t, err)
			assert.Empty(t, list)
		})
	}
}

func TestStylistNamedPlan_PlanSizeLimit(t *testing.T) {
	t.Parallel()
	wrap := func(total int) string {
		const pre, post = `{"a":"`, `"}`
		return pre + strings.Repeat("x", total-len(pre)-len(post)) + post
	}

	app, _ := newTestApp(t)
	_, err := app.StylistSaveNamedPlan("Большой", wrap(512*1024+1), false)
	assert.Error(t, err)
	list, err := app.StylistPlans()
	require.NoError(t, err)
	assert.Empty(t, list)

	_, err = app.StylistSaveNamedPlan("Большой", wrap(1024*1024), false)
	assert.Error(t, err)

	_, err = app.StylistSaveNamedPlan("Почти предел", wrap(500*1024), false)
	assert.NoError(t, err)
}

func TestStylistNamedPlan_NameSpacesCollapsed(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)
	_, err := app.StylistSaveNamedPlan("  Осень   в    парке  ", `{}`, false)
	require.NoError(t, err)
	list, err := app.StylistPlans()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Осень в парке", list[0].Name)
}

func TestStylistNamedPlan_DuplicateAndReplace(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)
	_, err := app.StylistSaveNamedPlan("Осень", `{"v":"old"}`, false)
	require.NoError(t, err)

	_, err = app.StylistSaveNamedPlan("осень", `{"v":"new"}`, false)
	assert.Error(t, err)
	list, err := app.StylistPlans()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, `{"v":"old"}`, list[0].Plan)

	_, err = app.StylistSaveNamedPlan("ОСЕНЬ", `{"v":"new"}`, true)
	require.NoError(t, err)
	list, err = app.StylistPlans()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, `{"v":"new"}`, list[0].Plan)
}

func TestStylistNamedPlan_RenameAndDelete(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)
	p, err := app.StylistSaveNamedPlan("Осень", `{}`, false)
	require.NoError(t, err)
	_, err = app.StylistSaveNamedPlan("Зима", `{}`, false)
	require.NoError(t, err)

	renamed, err := app.StylistRenamePlan(p.ID, "  Весна   в   саду ")
	require.NoError(t, err)
	assert.Equal(t, "Весна в саду", renamed.Name)

	_, err = app.StylistRenamePlan(p.ID, "зима")
	assert.Error(t, err)

	_, err = app.StylistRenamePlan(999, "Лето")
	assert.Error(t, err)

	require.NoError(t, app.StylistDeletePlan(p.ID))
	assert.Error(t, app.StylistDeletePlan(p.ID))

	list, err := app.StylistPlans()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Зима", list[0].Name)
}

func TestStylistNamedPlan_RenameInvalidName(t *testing.T) {
	t.Parallel()
	tests := []string{"", "   ", strings.Repeat("я", 81)}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			app, _ := newTestApp(t)
			p, err := app.StylistSaveNamedPlan("Осень", `{}`, false)
			require.NoError(t, err)
			_, err = app.StylistRenamePlan(p.ID, in)
			assert.Error(t, err)
			list, err := app.StylistPlans()
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, "Осень", list[0].Name)
		})
	}
}
