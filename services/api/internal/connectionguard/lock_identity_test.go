package connectionguard

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type lockIdentityQuery func(context.Context, string, ...any) pgx.Row

func (query lockIdentityQuery) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return query(ctx, sql, args...)
}

type lockIdentityRow func(...any) error

func (row lockIdentityRow) Scan(dest ...any) error { return row(dest...) }

func TestCanonicalLockKey(t *testing.T) {
	const validation = `SELECT pg_input_is_valid($1::text,'uuid')`
	const cast = `SELECT $1::text::uuid::text`
	const canonical = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	const alias = "{A0EEBC99-9C0B4EF8-BB6D6BB9BD380A11}"
	const composite = "portfolio-reconciliation:Owner-ABC:Account-DEF\t "
	validationErr := errors.New("validation query failed")
	castErr := errors.New("UUID cast query failed")
	type step struct {
		sql   string
		value any
		err   error
	}
	for _, tt := range []struct {
		name    string
		key     string
		steps   []step
		want    string
		wantErr error
	}{
		{"composite remains byte-identical without cast", composite, []step{{validation, false, nil}}, composite, nil},
		{"malformed UUID remains raw without cast", "{not-a-uuid}", []step{{validation, false, nil}}, "{not-a-uuid}", nil},
		{"empty key remains raw without cast", "", []step{{validation, false, nil}}, "", nil},
		{"valid UUID uses database canonical result", alias, []step{{validation, true, nil}, {cast, canonical, nil}}, canonical, nil},
		{"canonical UUID is unchanged", canonical, []step{{validation, true, nil}, {cast, canonical, nil}}, canonical, nil},
		{"validation error returns no key", alias, []step{{validation, nil, validationErr}}, "", validationErr},
		{"validation cancellation returns no key", alias, []step{{validation, nil, context.Canceled}}, "", context.Canceled},
		{"cast error returns no key", alias, []step{{validation, true, nil}, {cast, nil, castErr}}, "", castErr},
		{"partial cast result with error returns no key", alias, []step{{validation, true, nil}, {cast, canonical, castErr}}, "", castErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			db := lockIdentityQuery(func(gotCtx context.Context, sql string, args ...any) pgx.Row {
				if calls >= len(tt.steps) {
					t.Fatalf("unexpected query, including forbidden non-UUID cast: %s", sql)
				}
				next := tt.steps[calls]
				calls++
				if gotCtx != ctx || sql != next.sql || len(args) != 1 || args[0] != tt.key {
					t.Fatalf("query did not preserve context, statement, and original key: %q %#v", sql, args)
				}
				return lockIdentityRow(func(dest ...any) error {
					if len(dest) != 1 {
						t.Fatalf("unexpected scan destination count: %d", len(dest))
					}
					switch value := next.value.(type) {
					case bool:
						*dest[0].(*bool) = value
					case string:
						*dest[0].(*string) = value
					}
					return next.err
				})
			})
			got, err := CanonicalLockKey(ctx, db, tt.key)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Fatalf("CanonicalLockKey = %q, %v; want %q, %v", got, err, tt.want, tt.wantErr)
			}
			if calls != len(tt.steps) {
				t.Fatalf("queries = %d, want %d", calls, len(tt.steps))
			}
		})
	}
}
