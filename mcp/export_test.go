package mcp

import "time"

// SetStateClock replaces the clock requestState is signed and checked with.
func SetStateClock(s *Server, now func() time.Time) {
	s.signer.now = now
}
