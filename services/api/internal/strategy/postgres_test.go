package strategy

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestInitializeErrorIdentifiesActiveCapitalClaims(t *testing.T) {
	for _, constraint := range []string{
		"strategy_one_active_account_idx",
		"strategy_one_active_bucket_idx",
		"strategy_one_active_reservation_bucket_idx",
		"strategy_capital_reservation_account_guard",
	} {
		code := "23505"
		if constraint == "strategy_capital_reservation_account_guard" {
			code = "23514"
		}
		err := initializeError(&pgconn.PgError{Code: code, ConstraintName: constraint})
		if !errors.Is(err, ErrAccountInUse) {
			t.Fatalf("constraint %q did not map to ErrAccountInUse: %v", constraint, err)
		}
	}
	if err := initializeError(&pgconn.PgError{Code: "23514", ConstraintName: "strategy_capital_reservation_basis_guard"}); !errors.Is(err, ErrCapitalReservation) {
		t.Fatalf("invalid reservation basis did not map to ErrCapitalReservation: %v", err)
	}
	if err := initializeError(&pgconn.PgError{Code: "23505", ConstraintName: "strategy_one_active_mandate_version_idx"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate mandate version did not remain a generic conflict: %v", err)
	}
	sentinel := errors.New("database unavailable")
	if err := initializeError(sentinel); !errors.Is(err, sentinel) {
		t.Fatalf("non-constraint error was replaced: %v", err)
	}
}
