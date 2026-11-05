package session

import (
	"context"
	"log"
	"time"
)

func (s *Service) WatchExternal(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lastItem, lastSessions int64
	seen := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		item, sessions, err := s.db.SessionFingerprint()
		if err != nil {
			log.Printf("[session] watch: %v", err)
			continue
		}
		if seen && (item != lastItem || sessions != lastSessions) {
			s.emit.Emit("session:created", map[string]any{"external": true})
		}
		lastItem, lastSessions, seen = item, sessions, true
	}
}
