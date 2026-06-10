// Package api exposes Sforza's REST API.
package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/FreshMag/sforza/internal/auth"
	"github.com/FreshMag/sforza/internal/config"
	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/store"
)

// Server wires storage, authentication and configuration into an HTTP API.
type Server struct {
	cfg    *config.Config
	stores *store.Stores
	authn  auth.Authenticator
	syncMu sync.Mutex
}

// New creates a Server.
func New(cfg *config.Config, stores *store.Stores, authn auth.Authenticator) *Server {
	return &Server{cfg: cfg, stores: stores, authn: authn}
}

// Router builds the HTTP routing table.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.withAuth, s.withTenant)

		// Self-service permission queries.
		r.Route("/me", func(r chi.Router) {
			r.Get("/operations", s.handleMyOperations)
			r.Get("/record-ids", s.handleMyRecordIDs)
			r.Get("/meta-operations", s.handleMyMetaOperations)
		})

		// Resource administration.
		r.With(s.requireMeta(model.MetaResourceRead)).Get("/resources", s.handleListResources)
		r.With(s.requireMeta(model.MetaResourceWrite)).Post("/resources", s.handleCreateResource)
		r.With(s.requireMeta(model.MetaResourceWrite)).Delete("/resources/{name}", s.handleDeleteResource)

		// Operation administration.
		r.With(s.requireMeta(model.MetaOperationRead)).Get("/operations", s.handleListOperations)
		r.With(s.requireMeta(model.MetaOperationWrite)).Post("/operations", s.handleCreateOperation)
		r.With(s.requireMeta(model.MetaOperationWrite)).Delete("/operations/{name}", s.handleDeleteOperation)

		// User queries and user-level permissions.
		r.With(s.requireMeta(model.MetaUserRead)).Get("/users", s.handleListUsers)
		r.With(s.requireMeta(model.MetaUserRead)).Get("/users/{sub}/operations", s.handleUserOperations)
		r.With(s.requireMeta(model.MetaUserRead)).Get("/users/{sub}/meta-operations", s.handleUserMetaOperations)
		r.With(s.requireMeta(model.MetaRoleRead)).Get("/users/{sub}/roles", s.handleUserRoles)
		r.With(s.requireMeta(model.MetaOperationAssign)).Put("/users/{sub}/permissions/{operation}", s.handleSetUserPermission)
		r.With(s.requireMeta(model.MetaOperationAssign)).Delete("/users/{sub}/permissions/{operation}", s.handleRemoveUserPermission)
		r.With(s.requireMeta(model.MetaOperationAssign)).Post("/users/{sub}/permissions/{operation}/ids", s.handleUserRestrictedIDs)

		// Role administration.
		r.With(s.requireMeta(model.MetaRoleRead)).Get("/roles", s.handleListRoles)
		r.With(s.requireMeta(model.MetaRoleRead)).Get("/roles/{name}", s.handleGetRole)
		r.With(s.requireMeta(model.MetaRoleWrite)).Post("/roles", s.handleCreateRole)
		r.With(s.requireMeta(model.MetaRoleWrite)).Put("/roles/{name}", s.handleRenameRole)
		r.With(s.requireMeta(model.MetaRoleWrite)).Delete("/roles/{name}", s.handleDeleteRole)
		r.With(s.requireMeta(model.MetaRoleAssign)).Post("/roles/{name}/assignments/{sub}", s.handleAssignRole)
		r.With(s.requireMeta(model.MetaRoleAssign)).Delete("/roles/{name}/assignments/{sub}", s.handleUnassignRole)
		r.With(s.requireMeta(model.MetaOperationAssign)).Put("/roles/{name}/permissions/{operation}", s.handleSetRolePermission)
		r.With(s.requireMeta(model.MetaOperationAssign)).Delete("/roles/{name}/permissions/{operation}", s.handleRemoveRolePermission)
		r.With(s.requireMeta(model.MetaOperationAssign)).Post("/roles/{name}/permissions/{operation}/ids", s.handleRoleRestrictedIDs)

		// Runtime refresh of the bootstrap configuration.
		r.With(s.requireMeta(
			model.MetaResourceWrite, model.MetaOperationWrite,
			model.MetaRoleWrite, model.MetaRoleAssign, model.MetaOperationAssign,
		)).Post("/admin/sync", s.handleSync)
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
