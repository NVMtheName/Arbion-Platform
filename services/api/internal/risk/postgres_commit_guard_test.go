package risk

import (
	"context"
	"errors"
	"testing"
)

func TestCommitGuardRejectsMissingIdentityBeforeDatabaseAccess(t *testing.T) {
	for _, ids := range [][3]string{{"", "account", "mandate"}, {"owner", "", "mandate"}, {"owner", "account", ""}} {
		if err := LockCircuitBreakersForCommit(context.Background(), nil, ids[0], ids[1], ids[2]); !errors.Is(err, ErrCommitGuardUnavailable) {
			t.Fatalf("missing identity accepted: %v", err)
		}
	}
}
