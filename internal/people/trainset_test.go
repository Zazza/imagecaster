package people

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

type rollbackAPI struct {
	*fakeAPI
	rollbacks []string
	person    *serverclient.Person
	err       error
}

func (r *rollbackAPI) RollbackPerson(_ context.Context, name string) (*serverclient.Person, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rollbacks = append(r.rollbacks, name)
	if r.err != nil {
		return nil, r.err
	}
	return r.person, nil
}

func TestMaxPhotos_Is100(t *testing.T) {
	assert.Equal(t, 100, MaxPhotos)
}

func TestMaxPhotos_FolderTrainingBoundary(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		wantErr bool
	}{
		{"100 accepted", 100, false},
		{"101 rejected", 101, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			dir := t.TempDir()
			makePhotos(t, dir, tt.n)
			api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}

			_, err := newLocalService(api).Train(context.Background(), dir, "anna", "", "woman", "Z-Image", false)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrPhotoCount), "want ErrPhotoCount, got %v", err)
				assert.Empty(t, api.submits)
				return
			}
			require.NoError(t, err)
			require.Len(t, api.submits, 1)
			assert.Len(t, api.submits[0].Photos, 100)
		})
	}
}

func TestMaxPhotos_IdentityBoundary(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		wantErr bool
	}{
		{"100 accepted", 100, false},
		{"101 rejected", 101, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			dir := t.TempDir()
			want := makePhotos(t, dir, tt.n)
			api := &fakeAPI{}

			_, err := newLocalService(api).AddIdentity(context.Background(), dir, "anna")

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrIdentityCount), "want ErrIdentityCount, got %v", err)
				assert.Empty(t, api.identityPhotos)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, want, api.identityPhotos)
		})
	}
}

func TestTrainSet_NotLocalSendsNothing(t *testing.T) {
	clearProxyEnv(t)
	oldDir, newDir := t.TempDir(), t.TempDir()
	makePhotos(t, oldDir, 12)
	makePhotos(t, newDir, 4)
	api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}
	s := New(api, func() string { return "http://8.8.8.8:8080" }, func() string { return "inst-1" })

	job, err := s.TrainSet(context.Background(), TrainSetRequest{
		Folder: oldDir, NewFolder: newDir, Name: "anna", Gender: "woman", Base: "Z-Image", Mode: "continue",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotLocal), "want ErrNotLocal, got %v", err)
	assert.Nil(t, job)
	assert.Zero(t, api.totalCalls())
}

func TestTrainSet_PhotoCount(t *testing.T) {
	tests := []struct {
		name    string
		oldN    int
		newN    int
		wantErr bool
	}{
		{name: "10 old, no new folder", oldN: 10, newN: -1},
		{name: "9 old, no new folder", oldN: 9, newN: -1, wantErr: true},
		{name: "6 old + 4 new = 10", oldN: 6, newN: 4},
		{name: "5 old + 4 new = 9", oldN: 5, newN: 4, wantErr: true},
		{name: "60 old + 40 new = 100", oldN: 60, newN: 40},
		{name: "60 old + 41 new = 101", oldN: 60, newN: 41, wantErr: true},
		{name: "101 old", oldN: 101, newN: -1, wantErr: true},
		{name: "empty both", oldN: 0, newN: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			oldDir := t.TempDir()
			makePhotos(t, oldDir, tt.oldN)
			newDir := ""
			if tt.newN >= 0 {
				newDir = t.TempDir()
				makePhotos(t, newDir, tt.newN)
			}
			api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}

			job, err := newLocalService(api).TrainSet(context.Background(), TrainSetRequest{
				Folder: oldDir, NewFolder: newDir, Name: "anna", Gender: "woman", Base: "Z-Image", Mode: "continue",
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrPhotoCount), "want ErrPhotoCount, got %v", err)
				assert.Nil(t, job)
				assert.Empty(t, api.submits)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, job)
			require.Len(t, api.submits, 1)
		})
	}
}

func TestTrainSet_SubmitsRequest(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		newN    int
		hold    bool
		display string
		want    string
	}{
		{name: "continue with new photos", mode: "continue", newN: 4, display: "Анна   Мария", want: "Анна Мария"},
		{name: "empty mode, no new folder", mode: "", newN: -1, hold: true, display: "Анна", want: "Анна"},
		{name: "new mode", mode: "new", newN: 3, display: "Анна \t Мария", want: "Анна Мария"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			oldDir := t.TempDir()
			wantOld := makePhotos(t, oldDir, 10)
			newDir := ""
			var wantNew []string
			if tt.newN >= 0 {
				newDir = t.TempDir()
				wantNew = makePhotos(t, newDir, tt.newN)
			}
			api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j7", Status: "queued"}}

			job, err := newLocalService(api).TrainSet(context.Background(), TrainSetRequest{
				Folder: oldDir, NewFolder: newDir, Name: "Anna", Display: tt.display,
				Gender: "woman", Base: "Z-Image", Mode: tt.mode, Hold: tt.hold,
			})

			require.NoError(t, err)
			require.NotNil(t, job)
			assert.Equal(t, "j7", job.ID)
			require.Len(t, api.submits, 1)
			req := api.submits[0]
			assert.Equal(t, "anna", req.Name)
			assert.Equal(t, "inst-1", req.Client)
			assert.Equal(t, "inst-1:train:anna", req.ClientRef)
			assert.Equal(t, tt.want, req.DisplayName)
			assert.Equal(t, "woman", req.Gender)
			assert.Equal(t, "Z-Image", req.Base)
			assert.Equal(t, tt.mode, req.Mode)
			assert.Equal(t, tt.hold, req.Hold)
			assert.ElementsMatch(t, wantOld, req.Photos)
			if len(wantNew) > 0 {
				assert.ElementsMatch(t, wantNew, req.NewPhotos)
			} else {
				assert.Empty(t, req.NewPhotos)
			}
		})
	}
}

func TestTrainSet_SubmitErrorReturned(t *testing.T) {
	clearProxyEnv(t)
	oldDir := t.TempDir()
	makePhotos(t, oldDir, 10)
	apiErr := &serverclient.TrainAPIError{Status: http.StatusConflict, Code: "busy"}
	api := &fakeAPI{submitErr: apiErr}

	job, err := newLocalService(api).TrainSet(context.Background(), TrainSetRequest{
		Folder: oldDir, Name: "anna", Gender: "woman", Base: "Z-Image",
	})

	require.Error(t, err)
	assert.Nil(t, job)
	var got *serverclient.TrainAPIError
	assert.True(t, errors.As(err, &got), "want *TrainAPIError, got %T: %v", err, err)
}

func TestRollback(t *testing.T) {
	t.Run("api supports rollback", func(t *testing.T) {
		api := &rollbackAPI{fakeAPI: &fakeAPI{}, person: &serverclient.Person{Name: "anna", Rounds: 1}}

		p, err := newLocalService(api).Rollback(context.Background(), "anna")

		require.NoError(t, err)
		require.NotNil(t, p)
		assert.Equal(t, "anna", p.Name)
		assert.Equal(t, []string{"anna"}, api.rollbacks)
	})
	t.Run("api error returned", func(t *testing.T) {
		api := &rollbackAPI{fakeAPI: &fakeAPI{}, err: &serverclient.TrainAPIError{Status: http.StatusConflict, Code: "no_previous"}}

		p, err := newLocalService(api).Rollback(context.Background(), "anna")

		require.Error(t, err)
		assert.Nil(t, p)
		var apiErr *serverclient.TrainAPIError
		require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
		assert.Equal(t, "no_previous", apiErr.Code)
	})
}

func TestAddedAfter(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	msk := time.FixedZone("MSK", 3*3600)
	tests := []struct {
		name    string
		addedAt string
		since   time.Time
		want    bool
	}{
		{"one ms later", "2026-11-20T10:00:00.001Z", since, true},
		{"one day later", "2026-11-21T09:00:00.000Z", since, true},
		{"equal", "2026-11-20T10:00:00.000Z", since, false},
		{"earlier", "2026-11-20T09:59:59.999Z", since, false},
		{"since in another zone, later", "2026-11-20T10:00:00.500Z", since.In(msk), true},
		{"since in another zone, earlier", "2026-11-20T09:00:00.000Z", since.In(msk), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, AddedAfter(tt.addedAt, tt.since))
		})
	}
}

func dirWidths(t *testing.T, dir string) []int {
	t.Helper()
	var out []int
	for _, p := range regularFiles(t, dir) {
		if filepath.Ext(p) == "" {
			continue
		}
		cfg, _ := decodeFile(t, p)
		out = append(out, cfg.Width)
	}
	sort.Ints(out)
	return out
}

func (e *setEnv) addSized(t *testing.T, key string, seed int64, w int) string {
	t.Helper()
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, seed, w, 480), Key: key, Confirm: true})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Photo)
	return res.Photo.AddedAt
}

func parseAdded(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	require.NoError(t, err, "added_at %q", s)
	return ts
}

type splitEnv struct {
	*setEnv
	firstAt string
}

func newSplitEnv(t *testing.T) *splitEnv {
	t.Helper()
	e := newSetEnv(t)
	first := e.addSized(t, "anna", 1, 600)
	e.addSized(t, "anna", 2, 640)
	e.addSized(t, "olga", 3, 900)
	time.Sleep(10 * time.Millisecond)
	e.addSized(t, "anna", 4, 680)
	e.addSized(t, "anna", 5, 720)
	return &splitEnv{setEnv: e, firstAt: first}
}

func (e *splitEnv) cutoff(t *testing.T) time.Time {
	t.Helper()
	list, err := e.set.List("anna")
	require.NoError(t, err)
	require.Len(t, list, 4)
	var times []time.Time
	for _, p := range list {
		times = append(times, parseAdded(t, p.AddedAt))
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	require.True(t, times[2].Sub(times[1]) >= 5*time.Millisecond, "photos must be apart in time: %v", times)
	return times[1]
}

func TestSplitFolder_NoSinceAllOld(t *testing.T) {
	e := newSplitEnv(t)

	oldDir, newDir, cleanup, err := e.set.SplitFolder("anna", e.tmp, nil)

	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()
	assert.Equal(t, "", newDir)
	assert.Equal(t, []int{600, 640, 680, 720}, dirWidths(t, oldDir))
}

func TestSplitFolder_SinceSplitsByAddedAt(t *testing.T) {
	e := newSplitEnv(t)
	since := e.cutoff(t)

	oldDir, newDir, cleanup, err := e.set.SplitFolder("anna", e.tmp, &since)

	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()
	require.NotEmpty(t, newDir)
	assert.Equal(t, []int{600, 640}, dirWidths(t, oldDir))
	assert.Equal(t, []int{680, 720}, dirWidths(t, newDir))
}

func TestSplitFolder_SinceBeforeAllEverythingNew(t *testing.T) {
	e := newSplitEnv(t)
	since := parseAdded(t, e.firstAt).Add(-time.Hour)

	oldDir, newDir, cleanup, err := e.set.SplitFolder("anna", e.tmp, &since)

	require.NoError(t, err)
	defer cleanup()
	require.NotEmpty(t, newDir)
	assert.Equal(t, []int{600, 640, 680, 720}, dirWidths(t, newDir))
	assert.Empty(t, dirWidths(t, oldDir))
}

func TestSplitFolder_SinceAfterAllEverythingOld(t *testing.T) {
	e := newSplitEnv(t)
	since := time.Now().UTC().Add(time.Hour)

	oldDir, newDir, cleanup, err := e.set.SplitFolder("anna", e.tmp, &since)

	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, []int{600, 640, 680, 720}, dirWidths(t, oldDir))
	if newDir != "" {
		assert.Empty(t, dirWidths(t, newDir))
	}
}

func TestSplitFolder_OtherPersonNeverIncluded(t *testing.T) {
	e := newSplitEnv(t)
	since := e.cutoff(t)

	for _, s := range []*time.Time{nil, &since} {
		oldDir, newDir, cleanup, err := e.set.SplitFolder("anna", e.tmp, s)
		require.NoError(t, err)
		all := dirWidths(t, oldDir)
		if newDir != "" {
			all = append(all, dirWidths(t, newDir)...)
		}
		assert.NotContains(t, all, 900)
		cleanup()
	}

	oldDir, newDir, cleanup, err := e.set.SplitFolder("olga", e.tmp, nil)
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "", newDir)
	assert.Equal(t, []int{900}, dirWidths(t, oldDir))
}

func TestSplitFolder_CleanupRemovesEverything(t *testing.T) {
	e := newSplitEnv(t)
	since := e.cutoff(t)

	oldDir, newDir, cleanup, err := e.set.SplitFolder("anna", e.tmp, &since)
	require.NoError(t, err)
	require.NotEmpty(t, newDir)
	require.NotEmpty(t, regularFiles(t, oldDir))
	require.NotEmpty(t, regularFiles(t, newDir))

	cleanup()

	for _, d := range []string{oldDir, newDir} {
		_, statErr := os.Stat(d)
		assert.True(t, os.IsNotExist(statErr), "%s must be removed, stat err %v", d, statErr)
	}
	assert.Empty(t, regularFiles(t, e.tmp))
	assert.Equal(t, 4, e.count(t, "anna"), "set itself must stay intact")
}

func TestSummary_AddedPerPhoto(t *testing.T) {
	e := newSplitEnv(t)

	sums, err := e.set.Summary()

	require.NoError(t, err)
	byKey := map[string]SetSummary{}
	for _, s := range sums {
		byKey[s.Key] = s
	}
	for _, key := range []string{"anna", "olga"} {
		list, err := e.set.List(key)
		require.NoError(t, err)
		var want []string
		for _, p := range list {
			want = append(want, p.AddedAt)
		}
		s, ok := byKey[key]
		require.True(t, ok, "summary for %s missing", key)
		assert.Len(t, s.Added, s.Count)
		assert.ElementsMatch(t, want, s.Added)
	}
}
