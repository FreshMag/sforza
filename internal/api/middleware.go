package api

import (
	"context"
	"errors"
	"net/http"

	"gorm.io/gorm"

	"github.com/francesco/sforza/internal/auth"
	"github.com/francesco/sforza/internal/service"
)

// HeaderTenantID selects the active tenant for the request.
const HeaderTenantID = "X-Tenant-ID"

type ctxKey int

const (
	ctxSub ctxKey = iota
	ctxTenantID
	ctxTenantDB
)

func subject(r *http.Request) string {
	sub, _ := r.Context().Value(ctxSub).(string)
	return sub
}

func tenantDB(r *http.Request) *gorm.DB {
	db, _ := r.Context().Value(ctxTenantDB).(*gorm.DB)
	return db
}

// withAuth authenticates the request and lazily provisions the user in the
// shared database.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub, err := s.authn.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if err := service.EnsureUser(s.stores.Shared, sub); err != nil {
			writeError(w, http.StatusInternalServerError, "provision user: "+err.Error())
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxSub, sub)))
	})
}

// withTenant resolves the X-Tenant-ID header to a tenant database.
func (s *Server) withTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderTenantID)
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing "+HeaderTenantID+" header")
			return
		}
		db, ok := s.stores.Tenant(id)
		if !ok {
			writeError(w, http.StatusNotFound, "unknown tenant "+id)
			return
		}
		ctx := context.WithValue(r.Context(), ctxTenantID, id)
		ctx = context.WithValue(ctx, ctxTenantDB, db)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireMeta authorizes the caller against Sforza's own meta operations:
// every listed operation must be held with FULL scope in the active tenant.
func (s *Server) requireMeta(operations ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, op := range operations {
				ok, err := service.Authorize(tenantDB(r), subject(r), op)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				if !ok {
					writeError(w, http.StatusForbidden, "missing permission "+op)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// httpStatus maps service errors to HTTP status codes.
func httpStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, service.ErrValidation):
		return http.StatusBadRequest
	case errors.Is(err, auth.ErrUnauthenticated):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
