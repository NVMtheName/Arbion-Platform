package strategy

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrCommitMandateWindowClosed = errors.New("pinned AI mandate is outside its effective window at non-live commit")

type aiCommitMandateWindow struct {
	from  time.Time
	until *time.Time
}

// Read the immutable version pinned to the instance, not the mutable draft or
// model-start clock. Both fields must be saved; explicit null means no end.
// The access guard checks this window on the same database wall clock as the
// other time-limited authority, after locks and immediately before commit.
func parseAICommitMandateWindow(from, until []byte) (*aiCommitMandateWindow, error) {
	var w aiCommitMandateWindow
	if len(from) == 0 || len(until) == 0 || json.Unmarshal(from, &w.from) != nil ||
		json.Unmarshal(until, &w.until) != nil || w.from.IsZero() ||
		(w.until != nil && (w.until.IsZero() || !w.until.After(w.from))) {
		return nil, ErrEvaluationConfigurationChanged
	}
	return &w, nil
}

func (w aiCommitMandateWindow) current(now time.Time) bool {
	return !now.IsZero() && !w.from.IsZero() && !now.Before(w.from) &&
		(w.until == nil || (w.until.After(w.from) && now.Before(*w.until)))
}
