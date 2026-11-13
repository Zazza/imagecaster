package preset

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"modernc.org/sqlite"
)

const (
	HistoryKindGenerate  = "generate"
	HistoryKindFromImage = "from_image"
	HistoryKindPipeline  = "pipeline"
	HistoryKindEdit      = "edit"
	HistoryKindStylist   = "stylist"
	HistoryKindEditor    = "editor"
	HistoryKindOther     = "other"

	HistoryShapePortrait  = "portrait"
	HistoryShapeLandscape = "landscape"
	HistoryShapeSquare    = "square"
	HistoryShapeUnknown   = "unknown"

	historyDefaultLimit = 120
	historyMaxLimit     = 500
	historyTimeLayout   = "2006-01-02 15:04:05"
)

type HistoryFilter struct {
	SessionIDs   []int64           `json:"session_ids"`
	Models       []string          `json:"models"`
	Labels       []string          `json:"labels"`
	Kinds        []string          `json:"kinds"`
	Shapes       []string          `json:"shapes"`
	Search       string            `json:"search"`
	DateFrom     string            `json:"date_from"`
	DateTo       string            `json:"date_to"`
	PeriodStarts map[string]string `json:"period_starts"`
	Offset       int               `json:"offset"`
	Limit        int               `json:"limit"`
}

type HistoryItem struct {
	SessionItem
	SessionName string `json:"session_name"`
	Kind        string `json:"kind"`
	ModelName   string `json:"model_name"`
	Label       string `json:"label"`
	Shape       string `json:"shape"`
}

type HistoryFacet struct {
	Value    string `json:"value"`
	Name     string `json:"name,omitempty"`
	Count    int    `json:"count"`
	Pipeline bool   `json:"pipeline,omitempty"`
}

type HistoryFacets struct {
	Sessions []HistoryFacet `json:"sessions"`
	Models   []HistoryFacet `json:"models"`
	Labels   []HistoryFacet `json:"labels"`
	Kinds    []HistoryFacet `json:"kinds"`
	Shapes   []HistoryFacet `json:"shapes"`
	Periods  []HistoryFacet `json:"periods"`
}

type HistoryPage struct {
	Items  []HistoryItem `json:"items"`
	Total  int           `json:"total"`
	Facets HistoryFacets `json:"facets"`
}

func init() {
	if err := sqlite.RegisterDeterministicScalarFunction("ic_lower", 1, icLower); err != nil {
		panic(fmt.Sprintf("register ic_lower: %v", err))
	}
}

func icLower(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	switch v := args[0].(type) {
	case string:
		return strings.ToLower(v), nil
	case []byte:
		return strings.ToLower(string(v)), nil
	default:
		return "", nil
	}
}

const historyBase = `
WITH h AS (
	SELECT si.id, si.session_id, si.file_name, si.thumb_name, si.source, si.prompt, si.negative_prompt,
		si.sampler, si.steps, si.cfg_scale, si.seed, si.denoising, si.width, si.height, si.is_preview,
		si.preset_id, si.is_active, si.created_at, si.meta,
		COALESCE(s.name, '') AS session_name,
		CASE
			WHEN si.source = 'generate' THEN 'generate'
			WHEN si.source = 'from-image' THEN 'from_image'
			WHEN si.source IN ('compound', 'compound-from-image') THEN 'pipeline'
			WHEN si.source IN ('edit', 'look', 'object', 'repair') THEN 'edit'
			WHEN si.source = 'stylist' THEN 'stylist'
			WHEN si.source = 'editor' THEN 'editor'
			ELSE 'other'
		END AS kind,
		COALESCE(CASE WHEN json_valid(si.meta) THEN json_extract(si.meta, '$.model') END, '') AS model_name,
		CASE WHEN si.source IN ('generate', 'from-image', 'compound', 'compound-from-image') THEN
			COALESCE(NULLIF(CASE WHEN json_valid(si.meta) THEN json_extract(si.meta, '$.label') END, ''), p.name, '')
		ELSE '' END AS label,
		CASE
			WHEN si.width <= 0 OR si.height <= 0 THEN 'unknown'
			WHEN si.width * 100 > si.height * 105 THEN 'landscape'
			WHEN si.height * 100 > si.width * 105 THEN 'portrait'
			ELSE 'square'
		END AS shape
	FROM session_items si
	LEFT JOIN sessions s ON s.id = si.session_id
	LEFT JOIN presets p ON p.id = si.preset_id
)`

type historyRow struct {
	id          int64
	sessionID   int64
	sessionName string
	createdAt   string
	kind        string
	model       string
	label       string
	shape       string
}

func normalizeHistoryTime(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	for _, layout := range []string{historyTimeLayout, time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC().Format(historyTimeLayout), nil
		}
	}
	return "", fmt.Errorf("invalid date %q", v)
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func historyScanWhere(f HistoryFilter) (string, []any) {
	var parts []string
	var args []any
	if q := strings.ToLower(strings.TrimSpace(f.Search)); q != "" {
		if isASCII(q) {
			parts = append(parts, `prompt LIKE ? ESCAPE '\'`)
			args = append(args, "%"+escapeLike(q)+"%")
		} else {
			parts = append(parts, "instr(ic_lower(prompt), ?) > 0")
			args = append(args, q)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

type historyMatcher struct {
	sessions map[int64]bool
	models   map[string]bool
	labels   map[string]bool
	kinds    map[string]bool
	shapes   map[string]bool
	from     string
	to       string
}

func toSet(vals []string) map[string]bool {
	if len(vals) == 0 {
		return nil
	}
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

func newHistoryMatcher(f HistoryFilter) (*historyMatcher, error) {
	from, err := normalizeHistoryTime(f.DateFrom)
	if err != nil {
		return nil, err
	}
	to, err := normalizeHistoryTime(f.DateTo)
	if err != nil {
		return nil, err
	}
	m := &historyMatcher{from: from, to: to, models: toSet(f.Models), labels: toSet(f.Labels), kinds: toSet(f.Kinds), shapes: toSet(f.Shapes)}
	if len(f.SessionIDs) > 0 {
		m.sessions = make(map[int64]bool, len(f.SessionIDs))
		for _, id := range f.SessionIDs {
			m.sessions[id] = true
		}
	}
	return m, nil
}

func (m *historyMatcher) match(r *historyRow, skip string) bool {
	if skip != "dates" && ((m.from != "" && r.createdAt < m.from) || (m.to != "" && r.createdAt >= m.to)) {
		return false
	}
	if skip != "sessions" && m.sessions != nil && !m.sessions[r.sessionID] {
		return false
	}
	if skip != "models" && m.models != nil && !m.models[r.model] {
		return false
	}
	if skip != "labels" && m.labels != nil && !m.labels[r.label] {
		return false
	}
	if skip != "kinds" && m.kinds != nil && !m.kinds[r.kind] {
		return false
	}
	if skip != "shapes" && m.shapes != nil && !m.shapes[r.shape] {
		return false
	}
	return true
}

func (d *DB) ListHistory(f HistoryFilter) (*HistoryPage, error) {
	if f.Limit <= 0 {
		f.Limit = historyDefaultLimit
	}
	if f.Limit > historyMaxLimit {
		f.Limit = historyMaxLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	m, err := newHistoryMatcher(f)
	if err != nil {
		return nil, err
	}
	periodStarts := map[string]string{}
	for k, v := range f.PeriodStarts {
		norm, err := normalizeHistoryTime(v)
		if err != nil {
			return nil, err
		}
		if norm != "" {
			periodStarts[k] = norm
		}
	}
	where, args := historyScanWhere(f)
	rows, err := d.db.Query(historyBase+` SELECT id, session_id, session_name, created_at, kind, model_name, label, shape FROM h`+where+
		` ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	var all []historyRow
	for rows.Next() {
		var r historyRow
		if err := rows.Scan(&r.id, &r.sessionID, &r.sessionName, &r.createdAt, &r.kind, &r.model, &r.label, &r.shape); err != nil {
			rows.Close()
			return nil, err
		}
		if norm, err := normalizeHistoryTime(r.createdAt); err == nil {
			r.createdAt = norm
		}
		all = append(all, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}

	var matched []int64
	for i := range all {
		if m.match(&all[i], "") {
			matched = append(matched, all[i].id)
		}
	}
	page := &HistoryPage{Items: []HistoryItem{}, Total: len(matched)}
	page.Facets, err = d.historyFacets(all, m, f, periodStarts)
	if err != nil {
		return nil, err
	}
	if f.Offset >= len(matched) {
		return page, nil
	}
	end := f.Offset + f.Limit
	if end > len(matched) {
		end = len(matched)
	}
	page.Items, err = d.historyItems(matched[f.Offset:end])
	if err != nil {
		return nil, err
	}
	return page, nil
}

func (d *DB) historyItems(ids []int64) ([]HistoryItem, error) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := d.db.Query(historyBase+` SELECT id, session_id, file_name, thumb_name, source, prompt, negative_prompt,
		sampler, steps, cfg_scale, seed, denoising, width, height, is_preview, preset_id, is_active, created_at, meta,
		session_name, kind, model_name, label, shape FROM h WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)
		ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]HistoryItem, 0, len(ids))
	for rows.Next() {
		var it HistoryItem
		var meta sql.NullString
		if err := rows.Scan(&it.ID, &it.SessionID, &it.FileName, &it.ThumbName, &it.Source, &it.Prompt, &it.NegativePrompt,
			&it.Sampler, &it.Steps, &it.CfgScale, &it.Seed, &it.Denoising, &it.Width, &it.Height, &it.IsPreview,
			&it.PresetID, &it.IsActive, &it.CreatedAt, &meta, &it.SessionName, &it.Kind, &it.ModelName, &it.Label, &it.Shape); err != nil {
			return nil, err
		}
		it.Meta = unmarshalSessionMeta(meta)
		items = append(items, it)
	}
	return items, rows.Err()
}

type facetAcc struct {
	order  []string
	facets map[string]*HistoryFacet
	newest map[string]string
}

func newFacetAcc() *facetAcc {
	return &facetAcc{facets: map[string]*HistoryFacet{}, newest: map[string]string{}}
}

func (a *facetAcc) add(value, name, createdAt string, pipeline bool) {
	fc, ok := a.facets[value]
	if !ok {
		fc = &HistoryFacet{Value: value, Name: name}
		a.facets[value] = fc
		a.order = append(a.order, value)
		a.newest[value] = createdAt
	}
	fc.Count++
	if pipeline {
		fc.Pipeline = true
	}
}

func (a *facetAcc) ensure(value, name string) {
	if _, ok := a.facets[value]; !ok {
		a.facets[value] = &HistoryFacet{Value: value, Name: name}
		a.order = append(a.order, value)
	}
}

func (a *facetAcc) list(byNewest bool) []HistoryFacet {
	out := make([]HistoryFacet, 0, len(a.order))
	for _, v := range a.order {
		out = append(out, *a.facets[v])
	}
	if byNewest {
		sort.SliceStable(out, func(i, j int) bool { return a.newest[out[i].Value] > a.newest[out[j].Value] })
	} else {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Count != out[j].Count {
				return out[i].Count > out[j].Count
			}
			return out[i].Value < out[j].Value
		})
	}
	return out
}

func (d *DB) historyFacets(all []historyRow, m *historyMatcher, f HistoryFilter, periodStarts map[string]string) (HistoryFacets, error) {
	sessions, models, labels, kinds, shapes := newFacetAcc(), newFacetAcc(), newFacetAcc(), newFacetAcc(), newFacetAcc()
	periodKeys := make([]string, 0, len(periodStarts))
	for k := range periodStarts {
		periodKeys = append(periodKeys, k)
	}
	sort.Slice(periodKeys, func(i, j int) bool { return periodStarts[periodKeys[i]] > periodStarts[periodKeys[j]] })
	periodCounts := make(map[string]int, len(periodKeys))
	allCount := 0
	for i := range all {
		r := &all[i]
		if m.match(r, "dates") {
			allCount++
			for _, k := range periodKeys {
				if r.createdAt >= periodStarts[k] {
					periodCounts[k]++
				}
			}
		}
		if m.match(r, "sessions") {
			sessions.add(strconv.FormatInt(r.sessionID, 10), r.sessionName, r.createdAt, false)
		}
		if m.match(r, "models") {
			models.add(r.model, "", r.createdAt, false)
		}
		if r.label != "" && m.match(r, "labels") {
			labels.add(r.label, "", r.createdAt, r.kind == HistoryKindPipeline)
		}
		if m.match(r, "kinds") {
			kinds.add(r.kind, "", r.createdAt, false)
		}
		if m.match(r, "shapes") {
			shapes.add(r.shape, "", r.createdAt, false)
		}
	}
	var missing []any
	for _, id := range f.SessionIDs {
		if _, ok := sessions.facets[strconv.FormatInt(id, 10)]; !ok {
			missing = append(missing, id)
		}
	}
	names := map[int64]string{}
	if len(missing) > 0 {
		rows, err := d.db.Query(`SELECT id, name FROM sessions WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(missing)), ",")+`)`, missing...)
		if err != nil {
			return HistoryFacets{}, err
		}
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return HistoryFacets{}, err
			}
			names[id] = name
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return HistoryFacets{}, err
		}
	}
	for _, id := range f.SessionIDs {
		sessions.ensure(strconv.FormatInt(id, 10), names[id])
	}
	for _, v := range f.Models {
		models.ensure(v, "")
	}
	for _, v := range f.Labels {
		labels.ensure(v, "")
	}
	for _, v := range f.Kinds {
		kinds.ensure(v, "")
	}
	for _, v := range f.Shapes {
		shapes.ensure(v, "")
	}
	periods := []HistoryFacet{{Value: "all", Count: allCount}}
	for _, k := range periodKeys {
		periods = append(periods, HistoryFacet{Value: k, Count: periodCounts[k]})
	}
	return HistoryFacets{
		Periods:  periods,
		Sessions: sessions.list(true),
		Models:   models.list(false),
		Labels:   labels.list(false),
		Kinds:    kinds.list(false),
		Shapes:   shapes.list(false),
	}, nil
}
