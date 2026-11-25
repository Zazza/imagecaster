package main

import (
	"encoding/json"
	"sync"

	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/queue"
)

const (
	stylistLastShootKey = "stylist_last_shoot"
	stylistFrameDeleted = "deleted"
)

var lastShootMu sync.Mutex

type stylistShootStore interface {
	stylistSettings
	ExistingSessionItems(ids []int64) (map[int64]bool, error)
}

func loadLastShoot(st stylistSettings) []StylistFrame {
	raw, err := st.GetSetting(stylistLastShootKey)
	if err != nil || raw == "" {
		return nil
	}
	var frames []StylistFrame
	if json.Unmarshal([]byte(raw), &frames) != nil {
		return nil
	}
	return frames
}

func saveLastShoot(st stylistSettings, frames []StylistFrame) error {
	if frames == nil {
		frames = []StylistFrame{}
	}
	raw, err := json.Marshal(frames)
	if err != nil {
		return err
	}
	return st.SetSetting(stylistLastShootKey, string(raw))
}

func rememberShoot(st stylistSettings, ids []int64) error {
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	frames := make([]StylistFrame, len(ids))
	for i, id := range ids {
		frames[i] = StylistFrame{JobID: id, Status: string(queue.StatusPending)}
	}
	return saveLastShoot(st, frames)
}

func lastShootIDs(st stylistSettings) []int64 {
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	frames := loadLastShoot(st)
	ids := make([]int64, 0, len(frames))
	for _, f := range frames {
		ids = append(ids, f.JobID)
	}
	return ids
}

func sameJobIDs(a []StylistFrame, b []StylistFrame) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].JobID != b[i].JobID {
			return false
		}
	}
	return true
}

func recordShootFrame(st stylistSettings, jobID int64, raw json.RawMessage) error {
	var fr mcp.ShootFrameResult
	if json.Unmarshal(raw, &fr) != nil || fr.ID <= 0 {
		return nil
	}
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	stored := loadLastShoot(st)
	for i := range stored {
		if stored[i].JobID == jobID {
			stored[i] = StylistFrame{JobID: jobID, Status: string(queue.StatusCompleted), ItemID: fr.ID, Shot: fr.Shot, FaceSwap: fr.FaceSwap, FacePass: fr.FacePass, Garment: fr.Garment, AnatomyWarning: fr.AnatomyWarning}
			return saveLastShoot(st, stored)
		}
	}
	return nil
}

func mergeLastShoot(st stylistShootStore, frames []StylistFrame) ([]StylistFrame, error) {
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	stored := loadLastShoot(st)
	byID := make(map[int64]StylistFrame, len(stored))
	for _, f := range stored {
		byID[f.JobID] = f
	}
	for i, f := range frames {
		s, ok := byID[f.JobID]
		if ok && f.ItemID == 0 && s.ItemID > 0 {
			f = s
		}
		if f.Status == stylistFrameGone && f.ItemID == 0 {
			if ok && (s.Status == string(queue.StatusCancelled) || s.Status == string(queue.StatusFailed)) {
				f = s
			} else {
				f.Status = stylistFrameDeleted
			}
		}
		frames[i] = f
	}
	ids := make([]int64, 0, len(frames))
	for _, f := range frames {
		if f.ItemID > 0 {
			ids = append(ids, f.ItemID)
		}
	}
	exists, err := st.ExistingSessionItems(ids)
	if err != nil {
		return nil, err
	}
	keep := make([]StylistFrame, 0, len(frames))
	for i, f := range frames {
		if f.ItemID > 0 && !exists[f.ItemID] {
			f.Status = stylistFrameDeleted
			frames[i] = f
		}
		if f.Status != stylistFrameDeleted {
			keep = append(keep, f)
		}
	}
	if sameJobIDs(stored, frames) {
		changed, _ := json.Marshal(keep)
		was, _ := json.Marshal(stored)
		if string(changed) != string(was) {
			err = saveLastShoot(st, keep)
		}
	}
	return frames, err
}
