package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

type styIDCall struct {
	sessionID int64
	image     string
	source    string
	isPreview bool
	meta      *preset.GenerationMeta
	returned  int64
}

type fakeItemSessions struct {
	*fakeSessions
	mu     sync.Mutex
	items  map[int64]*preset.SessionItem
	images map[int64]string
	active int64
	reads  []int64
	adds   []styIDCall
	nextID int64
}

func newFakeItemSessions() *fakeItemSessions {
	return &fakeItemSessions{fakeSessions: &fakeSessions{}, items: map[int64]*preset.SessionItem{}, images: map[int64]string{}, nextID: 500}
}

func (f *fakeItemSessions) put(item *preset.SessionItem, img []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[item.ID] = item
	f.images[item.ID] = base64.StdEncoding.EncodeToString(img)
}

func (f *fakeItemSessions) GetActiveSessionItem() (*preset.SessionItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it, ok := f.items[f.active]
	if !ok {
		return nil, nil
	}
	cp := *it
	return &cp, nil
}

func (f *fakeItemSessions) SessionItem(id int64) (*preset.SessionItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it, ok := f.items[id]
	if !ok {
		return nil, nil
	}
	cp := *it
	return &cp, nil
}

func (f *fakeItemSessions) GetSessionImage(id int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, id)
	img, ok := f.images[id]
	if !ok {
		return "", fmt.Errorf("session item not found")
	}
	return img, nil
}

func (f *fakeItemSessions) AddToSessionID(sessionID int64, imageBase64 string, info json.RawMessage, source string, isPreview bool, meta *preset.GenerationMeta) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := f.nextID
	if sessionID <= 0 {
		id = 0
	}
	f.adds = append(f.adds, styIDCall{sessionID: sessionID, image: imageBase64, source: source, isPreview: isPreview, meta: meta, returned: id})
	return id
}

func (f *fakeItemSessions) idCalls() []styIDCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]styIDCall(nil), f.adds...)
}

func (f *fakeItemSessions) imageReads() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.reads...)
}

const (
	styItemID      = int64(42)
	styItemSession = int64(7)
)

func styItemSess() (*fakeItemSessions, []byte) {
	sess := newFakeItemSessions()
	data := styPhotoPNG(200, 300, color.NRGBA{R: 128, G: 128, B: 128, A: 255})
	sess.put(&preset.SessionItem{ID: styItemID, SessionID: styItemSession, Source: "sd", Width: 200, Height: 300}, data)
	sess.active = styItemID
	return sess, data
}

func styRetouchPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 300))
	skin := color.NRGBA{R: 120, G: 100, B: 120, A: 255}
	shade := color.NRGBA{R: 70, G: 55, B: 65, A: 255}
	for y := 0; y < 300; y++ {
		for x := 0; x < 200; x++ {
			c := skin
			ny := (float64(y) + 0.5) / 300
			nx := (float64(x) + 0.5) / 200
			if ny >= 0.418 && ny < 0.44 && (math.Abs(nx-0.38) < 0.045 || math.Abs(nx-0.62) < 0.045) {
				c = shade
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func styNum(t *testing.T, m map[string]any, key string) int64 {
	t.Helper()
	v, ok := m[key].(float64)
	require.True(t, ok, "response must have numeric %s: %v", key, m)
	return int64(v)
}

func TestStylistTry_Item(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		item any
	}{
		{name: "active", item: "active"},
		{name: "numeric id", item: float64(styItemID)},
		{name: "id as string", item: strconv.FormatInt(styItemID, 10)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{editPNG: styMossPortrait}
			sess, data := styItemSess()
			s, _ := styNewServer(t, eng, sess)

			m, out := styCall(t, s, "stylist_try", map[string]any{"item": tt.item, "look": map[string]any{"outfit": "a navy suit"}})

			edits := eng.editCalls()
			require.NotEmpty(t, edits)
			for i, c := range edits {
				assert.Equal(t, data, styDecodeB64(t, c.image), "edit %d must use the session item image", i)
			}
			assert.Empty(t, sess.namedCalls(), "variants must not go to a named session")
			calls := sess.idCalls()
			require.Len(t, calls, len(edits))
			var ids []int64
			for i, c := range calls {
				assert.Equal(t, styItemSession, c.sessionID, "variant %d session", i)
				assert.Equal(t, "stylist", c.source)
				require.NotNil(t, c.meta, "variant %d meta", i)
				assert.Equal(t, styItemID, c.meta.SourceItem, "variant %d source item", i)
				ids = append(ids, c.returned)
			}
			assert.Equal(t, styItemID, styNum(t, m, "source_item"), out)
			assert.Equal(t, styItemSession, styNum(t, m, "session_id"), out)
			assert.Equal(t, ids, styIDs(styVariants(t, m)))
		})
	}
}

func TestStylistTry_BadItem(t *testing.T) {
	t.Parallel()
	look := map[string]any{"outfit": "a navy suit"}
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "missing id", args: map[string]any{"item": float64(9999), "look": look}},
		{name: "word", args: map[string]any{"item": "foo", "look": look}},
		{name: "negative", args: map[string]any{"item": float64(-1), "look": look}},
		{name: "fraction", args: map[string]any{"item": 1.5, "look": look}},
		{name: "zero", args: map[string]any{"item": float64(0), "look": look}},
		{name: "photo_path and item", args: map[string]any{"item": "active", "photo_path": "/tmp/whatever.png", "look": look}},
		{name: "neither photo_path nor item", args: map[string]any{"look": look}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{editPNG: styMossPortrait}
			sess, _ := styItemSess()
			s, _ := styNewServer(t, eng, sess)

			_, err := styExec(s, "stylist_try", tt.args)

			require.Error(t, err)
			assert.Empty(t, eng.editCalls())
			assert.Empty(t, sess.idCalls())
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestStylistTry_BothPhotoAndItemWithRealFile(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: styMossPortrait}
	sess, _ := styItemSess()
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePortrait(t)

	_, err := styExec(s, "stylist_try", map[string]any{"item": "active", "photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}})

	require.Error(t, err)
	assert.Empty(t, eng.editCalls())
}

func TestStylistTry_ItemIsStylistResult(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: styMossPortrait}
	sess, _ := styItemSess()
	sess.put(&preset.SessionItem{ID: 77, SessionID: styItemSession, Source: "stylist", Meta: &preset.GenerationMeta{SourceItem: 4242}},
		styPhotoPNG(200, 300, color.NRGBA{R: 100, G: 100, B: 100, A: 255}))
	s, _ := styNewServer(t, eng, sess)

	_, err := styExec(s, "stylist_try", map[string]any{"item": float64(77), "look": map[string]any{"outfit": "a navy suit"}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "4242")
	assert.Empty(t, eng.editCalls())
	assert.Empty(t, sess.idCalls())
}

func TestStylistTry_ItemIsStylistResultWithoutSource(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: styMossPortrait}
	sess, _ := styItemSess()
	sess.put(&preset.SessionItem{ID: 78, SessionID: styItemSession, Source: "stylist"},
		styPhotoPNG(200, 300, color.NRGBA{R: 100, G: 100, B: 100, A: 255}))
	s, _ := styNewServer(t, eng, sess)

	_, err := styExec(s, "stylist_try", map[string]any{"item": float64(78), "look": map[string]any{"outfit": "a navy suit"}})

	require.Error(t, err)
	assert.Empty(t, eng.editCalls())
	assert.Empty(t, sess.idCalls())
}

func TestStylistTools_ItemWithoutItemSessions(t *testing.T) {
	t.Parallel()
	for _, tool := range []string{"stylist_try", "stylist_describe"} {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess := &fakeSessions{}
			s, _ := styNewServer(t, eng, sess)

			var err error
			require.NotPanics(t, func() {
				_, err = styExec(s, tool, map[string]any{"item": "active", "look": map[string]any{"outfit": "a navy suit"}})
			})

			require.Error(t, err)
			assert.Empty(t, eng.editCalls())
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestStylistTry_RetouchOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		retouch map[string]any
	}{
		{name: "under eyes", retouch: map[string]any{"under_eyes": true}},
		{name: "light", retouch: map[string]any{"light": true}},
		{name: "eye color", retouch: map[string]any{"eye_color": "blue"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess, _ := styItemSess()
			sess.put(&preset.SessionItem{ID: styItemID, SessionID: styItemSession, Source: "sd", Width: 200, Height: 300}, styRetouchPNG())
			s, _ := styNewServer(t, eng, sess)

			m, out := styCall(t, s, "stylist_try", map[string]any{"item": "active", "retouch": tt.retouch})

			assert.Empty(t, eng.editCalls(), "retouch only must not call Klein")
			calls := sess.idCalls()
			require.Len(t, calls, 1, out)
			assert.Equal(t, styItemSession, calls[0].sessionID)
			require.NotNil(t, calls[0].meta)
			assert.Equal(t, styItemID, calls[0].meta.SourceItem)
			assert.Len(t, styVariants(t, m), 1)
		})
	}
}

func TestStylistTry_EmptyLookAndRetouch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "both missing", args: map[string]any{"item": "active"}},
		{name: "both empty", args: map[string]any{"item": "active", "look": map[string]any{}, "retouch": map[string]any{}}},
		{name: "retouch all false", args: map[string]any{"item": "active", "retouch": map[string]any{"under_eyes": false, "teeth": false, "eye_color": ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess, _ := styItemSess()
			s, _ := styNewServer(t, eng, sess)

			_, err := styExec(s, "stylist_try", tt.args)

			require.Error(t, err)
			assert.Empty(t, eng.editCalls())
			assert.Empty(t, sess.idCalls())
		})
	}
}

func TestStylistTry_UnknownEyeColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		look map[string]any
	}{
		{name: "retouch only", look: nil},
		{name: "with look", look: map[string]any{"outfit": "a navy suit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess, _ := styItemSess()
			s, _ := styNewServer(t, eng, sess)
			args := map[string]any{"item": "active", "retouch": map[string]any{"eye_color": "purple-ish"}}
			if tt.look != nil {
				args["look"] = tt.look
			}

			_, err := styExec(s, "stylist_try", args)

			require.Error(t, err)
			assert.Empty(t, sess.imageReads(), "photo must not be read")
			assert.Empty(t, eng.editCalls())
			assert.Empty(t, eng.faceCalls())
			assert.Empty(t, eng.describeCalls())
			assert.Empty(t, sess.idCalls())
		})
	}
}

func TestStylistDescribe_Item(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		faceErr error
	}{
		{name: "face found"},
		{name: "face not found", faceErr: fmt.Errorf("no face found")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{faceErr: tt.faceErr}
			sess, data := styItemSess()
			s, _ := styNewServer(t, eng, sess)

			m, out := styCall(t, s, "stylist_describe", map[string]any{"item": "active"})

			assert.Equal(t, styItemID, styNum(t, m, "source_item"), out)
			photo, ok := m["photo"].(string)
			require.True(t, ok, "photo must be a string: %s", out)
			assert.NotEmpty(t, photo)
			if tt.faceErr != nil {
				low := strings.ToLower(photo)
				assert.True(t, strings.Contains(low, "face") || strings.Contains(low, "лиц"), "photo must mention the face: %s", photo)
			}
			calls := eng.describeCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, data, styDecodeB64(t, calls[0].image))
		})
	}
}

func styRPC(t *testing.T, s *Server, method string, params any) map[string]any {
	t.Helper()
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		req.Params = raw
	}
	raw, err := json.Marshal(s.dispatch(req))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func TestPrompts(t *testing.T) {
	t.Parallel()

	t.Run("initialize advertises prompts", func(t *testing.T) {
		t.Parallel()
		s, _ := styNewServer(t, nil, nil)
		m := styRPC(t, s, "initialize", map[string]any{})
		res, ok := m["result"].(map[string]any)
		require.True(t, ok, "%v", m)
		caps, ok := res["capabilities"].(map[string]any)
		require.True(t, ok, "%v", res)
		assert.Contains(t, caps, "prompts")
	})

	t.Run("list has stylist and photo_editor prompts", func(t *testing.T) {
		t.Parallel()
		s, _ := styNewServer(t, nil, nil)
		m := styRPC(t, s, "prompts/list", nil)
		res, ok := m["result"].(map[string]any)
		require.True(t, ok, "%v", m)
		prompts, ok := res["prompts"].([]any)
		require.True(t, ok, "%v", res)
		require.Len(t, prompts, 2)
		var names []any
		for _, raw := range prompts {
			p, ok := raw.(map[string]any)
			require.True(t, ok)
			names = append(names, p["name"])
		}
		assert.ElementsMatch(t, []any{"stylist", "photo_editor"}, names)
	})

	tests := []struct {
		name    string
		params  map[string]any
		wantErr bool
	}{
		{name: "get stylist with active photo", params: map[string]any{"name": "stylist", "arguments": map[string]any{"photo": "active"}}},
		{name: "get unknown prompt", params: map[string]any{"name": "other"}, wantErr: true},
		{name: "get without name", params: map[string]any{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _ := styNewServer(t, nil, nil)
			m := styRPC(t, s, "prompts/get", tt.params)
			if tt.wantErr {
				assert.NotNil(t, m["error"], "%v", m)
				assert.Nil(t, m["result"])
				return
			}
			require.Nil(t, m["error"], "%v", m)
			res, ok := m["result"].(map[string]any)
			require.True(t, ok, "%v", m)
			msgs, ok := res["messages"].([]any)
			require.True(t, ok, "%v", res)
			require.NotEmpty(t, msgs)
			msg, ok := msgs[0].(map[string]any)
			require.True(t, ok)
			content, ok := msg["content"].(map[string]any)
			require.True(t, ok, "%v", msg)
			text, _ := content["text"].(string)
			assert.Contains(t, text, "active")
		})
	}
}
