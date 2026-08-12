package authorization

import (
	"context"
	"testing"
)

type memoryStore struct{ users map[string]User }

func (m *memoryStore) List(context.Context) ([]User, error) {
	o := []User{}
	for _, u := range m.users {
		o = append(o, u)
	}
	return o, nil
}
func (m *memoryStore) Get(_ context.Context, id string) (User, error) { return m.users[id], nil }
func (m *memoryStore) SetRole(_ context.Context, id string, r Role) (Role, error) {
	u := m.users[id]
	old := u.Role
	u.Role = r
	m.users[id] = u
	return old, nil
}
func (m *memoryStore) SetEntitlement(_ context.Context, id string, e Entitlement, b bool, _ string) (Entitlement, error) {
	u := m.users[id]
	old := u.Entitlement
	u.Entitlement = e
	u.BillingRequired = b
	m.users[id] = u
	return old, nil
}
func (m *memoryStore) BootstrapFounder(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}

type noAudit struct{}

func (noAudit) Record(context.Context, *string, string, map[string]any) error { return nil }
func TestRolesAndEntitlementsAreIndependent(t *testing.T) {
	admin := Principal{UserID: "a", Role: RoleAdmin, Entitlement: EntitlementFree}
	if RequireAdmin(admin) != nil || CanUseNeuralEngine(admin) {
		t.Fatal("admin role granted product capability")
	}
	beta := Principal{UserID: "b", Role: RoleUser, Entitlement: EntitlementPremium}
	if RequireAdmin(beta) == nil || !CanUseNeuralEngine(beta) {
		t.Fatal("entitlement granted admin authority")
	}
	founder := Principal{UserID: "f", Role: RoleSuperadmin, Entitlement: EntitlementFounder, BillingRequired: false}
	if !CanUseAutomation(founder) || founder.BillingRequired {
		t.Fatal("founder policy failed")
	}
}
func TestFounderProtectionsAndManagement(t *testing.T) {
	m := &memoryStore{users: map[string]User{"f": {ID: "f", Role: RoleSuperadmin, Entitlement: EntitlementFounder}, "u": {ID: "u", Role: RoleUser, Entitlement: EntitlementFree}}}
	s := NewService(m, noAudit{})
	root := Principal{UserID: "f", Role: RoleSuperadmin}
	if _, e := s.SetRole(context.Background(), root, "f", RoleUser); e == nil {
		t.Fatal("founder demoted")
	}
	if _, e := s.SetEntitlement(context.Background(), root, "f", EntitlementFree, false); e == nil {
		t.Fatal("founder stripped")
	}
	if u, e := s.SetRole(context.Background(), root, "u", RoleAdmin); e != nil || u.Role != RoleAdmin {
		t.Fatal("normal user not managed")
	}
	admin := Principal{UserID: "x", Role: RoleAdmin}
	if _, e := s.SetRole(context.Background(), admin, "u", RoleSuperadmin); e == nil {
		t.Fatal("admin granted superadmin")
	}
}
