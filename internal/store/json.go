package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/FreshMag/sforza/internal/model"
)

// The JSON store keeps the whole dataset in memory, guarded by a mutex, and
// rewrites the backing file atomically (temp file + rename) on every
// mutation. It is meant for development, tests and small single-node
// deployments; the file layout mirrors the bootstrap YAML, so it stays
// human-readable and diffable.

func loadJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read json store %q: %w", path, err)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("parse json store %q: %w", path, err)
	}
	return nil
}

func saveJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sforza-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func insertSorted(list []string, v string) []string {
	i, found := slices.BinarySearch(list, v)
	if found {
		return list
	}
	return slices.Insert(list, i, v)
}

func removeString(list []string, v string) ([]string, bool) {
	i := slices.Index(list, v)
	if i < 0 {
		return list, false
	}
	return slices.Delete(list, i, i+1), true
}

// --- Shared implementation ---

type jsonSharedData struct {
	Users      []string          `json:"users"`
	Resources  []string          `json:"resources"`
	Operations map[string]string `json:"operations"` // name -> resource
}

type jsonShared struct {
	mu   sync.Mutex
	path string
	data jsonSharedData
}

func openJSONShared(path string) (*jsonShared, error) {
	s := &jsonShared{path: path, data: jsonSharedData{Operations: map[string]string{}}}
	if err := loadJSON(path, &s.data); err != nil {
		return nil, err
	}
	if s.data.Operations == nil {
		s.data.Operations = map[string]string{}
	}
	return s, nil
}

func (s *jsonShared) persist() error { return saveJSON(s.path, &s.data) }

func (s *jsonShared) EnsureUser(sub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.data.Users, sub) {
		return nil
	}
	s.data.Users = insertSorted(s.data.Users, sub)
	return s.persist()
}

func (s *jsonShared) ListUsers() ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := make([]User, 0, len(s.data.Users))
	for _, sub := range s.data.Users {
		users = append(users, User{Sub: sub})
	}
	return users, nil
}

func (s *jsonShared) CreateResource(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.data.Resources, name) {
		return fmt.Errorf("resource %q: %w", name, ErrConflict)
	}
	s.data.Resources = insertSorted(s.data.Resources, name)
	return s.persist()
}

func (s *jsonShared) EnsureResource(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.data.Resources, name) {
		return nil
	}
	s.data.Resources = insertSorted(s.data.Resources, name)
	return s.persist()
}

func (s *jsonShared) DeleteResource(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rest, ok := removeString(s.data.Resources, name)
	if !ok {
		return fmt.Errorf("resource %q: %w", name, ErrNotFound)
	}
	s.data.Resources = rest
	for op, res := range s.data.Operations {
		if res == name {
			delete(s.data.Operations, op)
		}
	}
	return s.persist()
}

func (s *jsonShared) ListResources() ([]Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resources := make([]Resource, 0, len(s.data.Resources))
	for _, name := range s.data.Resources {
		resources = append(resources, Resource{Name: name})
	}
	return resources, nil
}

func (s *jsonShared) CreateOperation(name, resource string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Operations[name]; ok {
		return fmt.Errorf("operation %q: %w", name, ErrConflict)
	}
	if !slices.Contains(s.data.Resources, resource) {
		s.data.Resources = insertSorted(s.data.Resources, resource)
	}
	s.data.Operations[name] = resource
	return s.persist()
}

func (s *jsonShared) EnsureOperation(name, resource string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Operations[name]; ok {
		return nil
	}
	if !slices.Contains(s.data.Resources, resource) {
		s.data.Resources = insertSorted(s.data.Resources, resource)
	}
	s.data.Operations[name] = resource
	return s.persist()
}

func (s *jsonShared) DeleteOperation(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Operations[name]; !ok {
		return fmt.Errorf("operation %q: %w", name, ErrNotFound)
	}
	delete(s.data.Operations, name)
	return s.persist()
}

func (s *jsonShared) ListOperations() ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operations := make([]Operation, 0, len(s.data.Operations))
	for name, resource := range s.data.Operations {
		operations = append(operations, Operation{Name: name, Resource: resource})
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].Name < operations[j].Name })
	return operations, nil
}

func (s *jsonShared) OperationExists(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data.Operations[name]
	return ok, nil
}

// --- Tenant implementation ---

type jsonRoleData struct {
	Permissions   map[string]string   `json:"permissions,omitempty"`    // operation -> scope
	RestrictedIDs map[string][]string `json:"restricted-ids,omitempty"` // operation -> record IDs
}

type jsonUserData struct {
	Roles         []string            `json:"roles,omitempty"`
	Permissions   map[string]string   `json:"permissions,omitempty"`
	RestrictedIDs map[string][]string `json:"restricted-ids,omitempty"`
}

type jsonTenantData struct {
	Roles map[string]*jsonRoleData `json:"roles"`
	Users map[string]*jsonUserData `json:"users"`
}

type jsonTenant struct {
	mu   sync.Mutex
	path string
	data jsonTenantData
}

func openJSONTenant(path string) (*jsonTenant, error) {
	t := &jsonTenant{path: path, data: jsonTenantData{
		Roles: map[string]*jsonRoleData{},
		Users: map[string]*jsonUserData{},
	}}
	if err := loadJSON(path, &t.data); err != nil {
		return nil, err
	}
	if t.data.Roles == nil {
		t.data.Roles = map[string]*jsonRoleData{}
	}
	if t.data.Users == nil {
		t.data.Users = map[string]*jsonUserData{}
	}
	return t, nil
}

func (t *jsonTenant) persist() error { return saveJSON(t.path, &t.data) }

func newJSONRole() *jsonRoleData {
	return &jsonRoleData{Permissions: map[string]string{}, RestrictedIDs: map[string][]string{}}
}

func (t *jsonTenant) role(name string) (*jsonRoleData, error) {
	r, ok := t.data.Roles[name]
	if !ok {
		return nil, fmt.Errorf("role %q: %w", name, ErrNotFound)
	}
	if r.Permissions == nil {
		r.Permissions = map[string]string{}
	}
	if r.RestrictedIDs == nil {
		r.RestrictedIDs = map[string][]string{}
	}
	return r, nil
}

func (t *jsonTenant) user(sub string) *jsonUserData {
	u, ok := t.data.Users[sub]
	if !ok {
		u = &jsonUserData{Permissions: map[string]string{}, RestrictedIDs: map[string][]string{}}
		t.data.Users[sub] = u
	}
	if u.Permissions == nil {
		u.Permissions = map[string]string{}
	}
	if u.RestrictedIDs == nil {
		u.RestrictedIDs = map[string][]string{}
	}
	return u
}

func (t *jsonTenant) CreateRole(name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.data.Roles[name]; ok {
		return fmt.Errorf("role %q: %w", name, ErrConflict)
	}
	t.data.Roles[name] = newJSONRole()
	return t.persist()
}

func (t *jsonTenant) EnsureRole(name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.data.Roles[name]; ok {
		return nil
	}
	t.data.Roles[name] = newJSONRole()
	return t.persist()
}

func (t *jsonTenant) RoleExists(name string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.data.Roles[name]
	return ok, nil
}

func (t *jsonTenant) RenameRole(name, newName string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, err := t.role(name)
	if err != nil {
		return err
	}
	if _, ok := t.data.Roles[newName]; ok {
		return fmt.Errorf("role %q: %w", newName, ErrConflict)
	}
	delete(t.data.Roles, name)
	t.data.Roles[newName] = r
	for _, u := range t.data.Users {
		if rest, ok := removeString(u.Roles, name); ok {
			u.Roles = insertSorted(rest, newName)
		}
	}
	return t.persist()
}

func (t *jsonTenant) DeleteRole(name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.data.Roles[name]; !ok {
		return fmt.Errorf("role %q: %w", name, ErrNotFound)
	}
	delete(t.data.Roles, name)
	for _, u := range t.data.Users {
		u.Roles, _ = removeString(u.Roles, name)
	}
	return t.persist()
}

func (t *jsonTenant) ListRoles() ([]Role, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	names := make([]string, 0, len(t.data.Roles))
	for name := range t.data.Roles {
		names = append(names, name)
	}
	sort.Strings(names)
	roles := make([]Role, 0, len(names))
	for _, name := range names {
		roles = append(roles, Role{Name: name})
	}
	return roles, nil
}

func (t *jsonTenant) AssignRole(sub, role string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.role(role); err != nil {
		return err
	}
	u := t.user(sub)
	if slices.Contains(u.Roles, role) {
		return nil
	}
	u.Roles = insertSorted(u.Roles, role)
	return t.persist()
}

func (t *jsonTenant) UnassignRole(sub, role string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.role(role); err != nil {
		return err
	}
	u := t.user(sub)
	rest, ok := removeString(u.Roles, role)
	if !ok {
		return fmt.Errorf("user %q has no role %q: %w", sub, role, ErrNotFound)
	}
	u.Roles = rest
	return t.persist()
}

func (t *jsonTenant) UserRoles(sub string) ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.data.Users[sub]
	if !ok {
		return []string{}, nil
	}
	return slices.Clone(u.Roles), nil
}

func (t *jsonTenant) SetRolePermission(role, operation string, scope model.Scope) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, err := t.role(role)
	if err != nil {
		return err
	}
	r.Permissions[operation] = string(scope)
	return t.persist()
}

func (t *jsonTenant) RemoveRolePermission(role, operation string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, err := t.role(role)
	if err != nil {
		return err
	}
	if _, ok := r.Permissions[operation]; !ok {
		return fmt.Errorf("role %q has no permission on %q: %w", role, operation, ErrNotFound)
	}
	delete(r.Permissions, operation)
	delete(r.RestrictedIDs, operation)
	return t.persist()
}

func (t *jsonTenant) RolePermissions(role string) ([]model.OperationScope, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, err := t.role(role)
	if err != nil {
		return nil, err
	}
	perms := make([]model.OperationScope, 0, len(r.Permissions))
	for op, scope := range r.Permissions {
		perms = append(perms, model.OperationScope{Operation: op, Scope: model.Scope(scope)})
	}
	sort.Slice(perms, func(i, j int) bool { return perms[i].Operation < perms[j].Operation })
	return perms, nil
}

func (t *jsonTenant) HasRolePermission(role, operation string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, err := t.role(role)
	if err != nil {
		return false, err
	}
	_, ok := r.Permissions[operation]
	return ok, nil
}

func (t *jsonTenant) SetUserPermission(sub, operation string, scope model.Scope) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.user(sub).Permissions[operation] = string(scope)
	return t.persist()
}

func (t *jsonTenant) RemoveUserPermission(sub, operation string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	u := t.user(sub)
	if _, ok := u.Permissions[operation]; !ok {
		return fmt.Errorf("user %q has no permission on %q: %w", sub, operation, ErrNotFound)
	}
	delete(u.Permissions, operation)
	delete(u.RestrictedIDs, operation)
	return t.persist()
}

func (t *jsonTenant) UserPermissions(sub string) ([]model.OperationScope, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.data.Users[sub]
	if !ok {
		return []model.OperationScope{}, nil
	}
	perms := make([]model.OperationScope, 0, len(u.Permissions))
	for op, scope := range u.Permissions {
		perms = append(perms, model.OperationScope{Operation: op, Scope: model.Scope(scope)})
	}
	sort.Slice(perms, func(i, j int) bool { return perms[i].Operation < perms[j].Operation })
	return perms, nil
}

func (t *jsonTenant) HasUserPermission(sub, operation string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.data.Users[sub]
	if !ok {
		return false, nil
	}
	_, has := u.Permissions[operation]
	return has, nil
}

func (t *jsonTenant) AddRoleRestrictedIDs(role, operation string, ids []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, err := t.role(role)
	if err != nil {
		return err
	}
	for _, id := range ids {
		r.RestrictedIDs[operation] = insertSorted(r.RestrictedIDs[operation], id)
	}
	return t.persist()
}

func (t *jsonTenant) RemoveRoleRestrictedIDs(role, operation string, ids []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, err := t.role(role)
	if err != nil {
		return err
	}
	for _, id := range ids {
		r.RestrictedIDs[operation], _ = removeString(r.RestrictedIDs[operation], id)
	}
	return t.persist()
}

func (t *jsonTenant) RoleRestrictedIDs(roles []string, operation string) ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []string{}
	seen := map[string]bool{}
	for _, name := range roles {
		r, ok := t.data.Roles[name]
		if !ok || r.RestrictedIDs == nil {
			continue
		}
		for _, id := range r.RestrictedIDs[operation] {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

func (t *jsonTenant) AddUserRestrictedIDs(sub, operation string, ids []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	u := t.user(sub)
	for _, id := range ids {
		u.RestrictedIDs[operation] = insertSorted(u.RestrictedIDs[operation], id)
	}
	return t.persist()
}

func (t *jsonTenant) RemoveUserRestrictedIDs(sub, operation string, ids []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	u := t.user(sub)
	for _, id := range ids {
		u.RestrictedIDs[operation], _ = removeString(u.RestrictedIDs[operation], id)
	}
	return t.persist()
}

func (t *jsonTenant) UserRestrictedIDs(sub, operation string) ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.data.Users[sub]
	if !ok {
		return []string{}, nil
	}
	return slices.Clone(u.RestrictedIDs[operation]), nil
}

func (t *jsonTenant) RoleGrants(roles []string) ([]RoleGrant, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	grants := []RoleGrant{}
	for _, name := range roles {
		r, ok := t.data.Roles[name]
		if !ok {
			continue
		}
		for op, scope := range r.Permissions {
			grants = append(grants, RoleGrant{Role: name, Operation: op, Scope: model.Scope(scope)})
		}
	}
	return grants, nil
}
