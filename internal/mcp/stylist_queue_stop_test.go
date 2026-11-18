package mcp

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestRunShootFrame_CancelUnnoticedByStepsNotSaved(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		facePass bool
	}{
		{name: "no face pass", facePass: false},
		{name: "face pass skipped without interrupt", facePass: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sess := &fakeSessions{}
			p := sqFrameParams()
			p.FacePass = tt.facePass

			_, err := runShootFrame(&fakeShooter{}, sess, &stubFaceEng{}, &fakeRefiner{}, p, func() bool { return true })

			require.Error(t, err)
			assert.True(t, generation.IsInterrupted(err))
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestRunShootFrame_NotStoppedSaves(t *testing.T) {
	t.Parallel()
	sess := &fakeSessions{}

	res, err := runShootFrame(&fakeShooter{}, sess, &stubFaceEng{}, nil, sqFrameParams(), func() bool { return false })

	require.NoError(t, err)
	require.Len(t, sess.namedCalls(), 1)
	assert.Equal(t, sess.namedCalls()[0].returned, res.ID)
}

func TestServiceStopped(t *testing.T) {
	t.Parallel()
	var s generation.Service
	assert.False(t, s.Stopped())
	ctx, cancel := context.WithCancel(context.Background())
	q, _ := s.ForQueue(ctx, nil, nil)
	assert.False(t, q.Stopped())
	cancel()
	assert.True(t, q.Stopped())
}

func TestRunShootFrame_ErrorWithoutServerURL(t *testing.T) {
	t.Parallel()
	sess := &fakeSessions{}
	err := &url.Error{Op: "Post", URL: "http://192.168.1.184:8080/api/sd/jobs", Err: errors.New("connection refused")}

	_, got := runShootFrame(&sqErrShooter{err: err}, sess, &stubFaceEng{}, nil, sqFrameParams(), nil)

	require.Error(t, got)
	assert.NotContains(t, got.Error(), "192.168")
	assert.Contains(t, got.Error(), "connection refused")
	assert.Empty(t, sess.namedCalls())
}

func TestShootPrompt_DefaultOutfit(t *testing.T) {
	t.Parallel()
	p := serverclient.Person{Lora: "anna"}
	assert.Contains(t, shootPrompt(p, "a woman", Look{}, "front", shootBackground), shootDefaultOutfit)
	withOutfit := shootPrompt(p, "a woman", Look{Outfit: "a navy suit"}, "front", shootBackground)
	assert.NotContains(t, withOutfit, shootDefaultOutfit)
	assert.Contains(t, withOutfit, "a navy suit")
}
