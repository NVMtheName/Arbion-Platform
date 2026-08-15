package authorization

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type scriptedRow struct {
	values []any
}

func (r scriptedRow) Scan(dest ...any) error {
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan destination count %d does not match value count %d", len(dest), len(r.values))
	}
	for i, value := range r.values {
		switch target := dest[i].(type) {
		case *string:
			*target = value.(string)
		case *Role:
			*target = value.(Role)
		case *Entitlement:
			*target = value.(Entitlement)
		case *bool:
			*target = value.(bool)
		default:
			return fmt.Errorf("unsupported scan destination %T", dest[i])
		}
	}
	return nil
}

type scriptedDB struct {
	rows     []scriptedRow
	execSQL  string
	execArgs []any
}

func (db *scriptedDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unexpected Query call")
}

func (db *scriptedDB) QueryRow(context.Context, string, ...any) pgx.Row {
	row := db.rows[0]
	db.rows = db.rows[1:]
	return row
}

func (db *scriptedDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execSQL = sql
	db.execArgs = args
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func TestBootstrapFounderUsesOneAtomicStatement(t *testing.T) {
	db := &scriptedDB{rows: []scriptedRow{
		{values: []any{"user-1", "nvm427@gmail.com", "Nick Maya", RoleUser, EntitlementFree, false}},
		{values: []any{"user-1", "nvm427@gmail.com", "Nick Maya", RoleSuperadmin, EntitlementFounder, false}},
	}}

	user, changed, err := NewPostgresStore(db).BootstrapFounder(context.Background(), "  NVM427@GMAIL.COM ")
	if err != nil {
		t.Fatal(err)
	}
	if !changed || user.Role != RoleSuperadmin || user.Entitlement != EntitlementFounder || user.BillingRequired {
		t.Fatalf("unexpected founder result: changed=%v user=%+v", changed, user)
	}
	if !strings.Contains(db.execSQL, "WITH updated_user AS") || strings.Contains(db.execSQL, ";") {
		t.Fatalf("bootstrap must use one atomic statement: %q", db.execSQL)
	}
	if len(db.execArgs) != 1 || db.execArgs[0] != "user-1" {
		t.Fatalf("unexpected bootstrap arguments: %#v", db.execArgs)
	}
}

func TestSetEntitlementUsesOneAtomicStatement(t *testing.T) {
	db := &scriptedDB{rows: []scriptedRow{{values: []any{EntitlementFree}}}}

	old, err := NewPostgresStore(db).SetEntitlement(context.Background(), "user-1", EntitlementPremium, true, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if old != EntitlementFree {
		t.Fatalf("unexpected old entitlement: %q", old)
	}
	if !strings.Contains(db.execSQL, "WITH revoked AS") || strings.Contains(db.execSQL, ";") {
		t.Fatalf("entitlement change must use one atomic statement: %q", db.execSQL)
	}
	if len(db.execArgs) != 4 || db.execArgs[0] != "user-1" || db.execArgs[1] != EntitlementPremium || db.execArgs[2] != "admin" || db.execArgs[3] != true {
		t.Fatalf("unexpected entitlement arguments: %#v", db.execArgs)
	}
}
