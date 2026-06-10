package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/francesco/sforza/internal/model"
	"github.com/francesco/sforza/internal/service"
)

// --- Self-service permission queries ---

func (s *Server) handleMyOperations(w http.ResponseWriter, r *http.Request) {
	ops, err := service.EffectiveOperations(tenantDB(r), subject(r))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ops)
}

func (s *Server) handleMyMetaOperations(w http.ResponseWriter, r *http.Request) {
	ops, err := service.MetaOperations(tenantDB(r), subject(r))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ops)
}

func (s *Server) handleMyRecordIDs(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("operations")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "missing operations query parameter (comma-separated)")
		return
	}
	ids, err := service.RecordIDs(tenantDB(r), subject(r), strings.Split(raw, ","))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ids)
}

// --- Resources ---

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	resources, err := service.ListResources(s.stores.Shared)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resources)
}

func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := service.CreateResource(s.stores.Shared, req.Name); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

func (s *Server) handleDeleteResource(w http.ResponseWriter, r *http.Request) {
	if err := service.DeleteResource(s.stores.Shared, chi.URLParam(r, "name")); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Operations ---

func (s *Server) handleListOperations(w http.ResponseWriter, r *http.Request) {
	operations, err := service.ListOperations(s.stores.Shared)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, operations)
}

func (s *Server) handleCreateOperation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := service.CreateOperation(s.stores.Shared, req.Name); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

func (s *Server) handleDeleteOperation(w http.ResponseWriter, r *http.Request) {
	if err := service.DeleteOperation(s.stores.Shared, chi.URLParam(r, "name")); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Users ---

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := service.ListUsers(s.stores.Shared)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleUserOperations(w http.ResponseWriter, r *http.Request) {
	ops, err := service.EffectiveOperations(tenantDB(r), chi.URLParam(r, "sub"))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ops)
}

func (s *Server) handleUserMetaOperations(w http.ResponseWriter, r *http.Request) {
	ops, err := service.MetaOperations(tenantDB(r), chi.URLParam(r, "sub"))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ops)
}

func (s *Server) handleUserRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := service.UserRoles(tenantDB(r), chi.URLParam(r, "sub"))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

// --- Roles ---

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := service.ListRoles(tenantDB(r))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (s *Server) handleGetRole(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	perms, err := service.RolePermissions(tenantDB(r), name)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	permissions := make([]model.OperationScope, 0, len(perms))
	for _, p := range perms {
		permissions = append(permissions, model.OperationScope{
			Operation: p.Operation, Scope: model.Scope(p.Scope),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "permissions": permissions})
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := service.CreateRole(tenantDB(r), req.Name); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

func (s *Server) handleRenameRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := service.RenameRole(tenantDB(r), chi.URLParam(r, "name"), req.Name); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": req.Name})
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if err := service.DeleteRole(tenantDB(r), chi.URLParam(r, "name")); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssignRole(w http.ResponseWriter, r *http.Request) {
	err := service.AssignRole(s.stores.Shared, tenantDB(r), chi.URLParam(r, "sub"), chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnassignRole(w http.ResponseWriter, r *http.Request) {
	err := service.UnassignRole(tenantDB(r), chi.URLParam(r, "sub"), chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Permission assignments ---

func (s *Server) handleSetRolePermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope model.Scope `json:"scope"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	err := service.SetRolePermission(s.stores.Shared, tenantDB(r),
		chi.URLParam(r, "name"), chi.URLParam(r, "operation"), req.Scope)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveRolePermission(w http.ResponseWriter, r *http.Request) {
	err := service.RemoveRolePermission(tenantDB(r), chi.URLParam(r, "name"), chi.URLParam(r, "operation"))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetUserPermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope model.Scope `json:"scope"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	err := service.SetUserPermission(s.stores.Shared, tenantDB(r),
		chi.URLParam(r, "sub"), chi.URLParam(r, "operation"), req.Scope)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveUserPermission(w http.ResponseWriter, r *http.Request) {
	err := service.RemoveUserPermission(tenantDB(r), chi.URLParam(r, "sub"), chi.URLParam(r, "operation"))
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Restricted IDs ---

type restrictedIDsRequest struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

func (s *Server) handleRoleRestrictedIDs(w http.ResponseWriter, r *http.Request) {
	var req restrictedIDsRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	err := service.UpdateRoleRestrictedIDs(tenantDB(r),
		chi.URLParam(r, "name"), chi.URLParam(r, "operation"), req.Add, req.Remove)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUserRestrictedIDs(w http.ResponseWriter, r *http.Request) {
	var req restrictedIDsRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	err := service.UpdateUserRestrictedIDs(tenantDB(r),
		chi.URLParam(r, "sub"), chi.URLParam(r, "operation"), req.Add, req.Remove)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Bootstrap refresh ---

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	files, err := service.LoadBootstrapFiles(s.cfg.Bootstrap.Files)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := service.Sync(s.stores, files); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced-files": len(files)})
}
