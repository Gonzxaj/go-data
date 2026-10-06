package api

import (
	"encoding/json"
	"errors"
	"go-data/internal/auth"
	"go-data/internal/users"
	"net/http"
	"strconv"
)

// UsersHandler is the admin-only CRUD over dashboard accounts, registered
// only when AUTH_MODE=session.
type UsersHandler struct {
	Store *users.Store
}

func (h *UsersHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/users", auth.RequireAdmin(h.list))
	mux.HandleFunc("POST /api/users", auth.RequireAdmin(h.create))
	mux.HandleFunc("PUT /api/users/{id}", auth.RequireAdmin(h.update))
	mux.HandleFunc("DELETE /api/users/{id}", auth.RequireAdmin(h.delete))
}

func (h *UsersHandler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo listar usuarios")
		return
	}
	writeJSON(w, list)
}

func (h *UsersHandler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string     `json:"username"`
		Password string     `json:"password"`
		Role     users.Role `json:"role"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	u, err := h.Store.Create(body.Username, body.Password, body.Role)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(u)
}

func (h *UsersHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Role     *users.Role `json:"role"`
		Active   *bool       `json:"active"`
		Password *string     `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	me, _ := auth.CurrentUser(r)
	keep := ""
	if id == me.ID {
		// An admin locking themselves out by accident is the likeliest way
		// to end up with no usable account; "last admin" alone doesn't
		// cover it when other admins exist but nobody remembers them.
		if (body.Role != nil && *body.Role != users.RoleAdmin) || (body.Active != nil && !*body.Active) {
			writeErr(w, http.StatusBadRequest, "no podés quitarte el rol de admin ni desactivarte a vos mismo")
			return
		}
		keep = auth.CurrentSession(r)
	}
	u, err := h.Store.Update(id, users.Update{Role: body.Role, Active: body.Active, Password: body.Password}, keep)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, u)
}

func (h *UsersHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if me, _ := auth.CurrentUser(r); id == me.ID {
		writeErr(w, http.StatusBadRequest, "no podés eliminar tu propio usuario")
		return
	}
	if err := h.Store.Delete(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "petición inválida")
		return false
	}
	return true
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, users.ErrUsernameTaken), errors.Is(err, users.ErrLastAdmin):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, users.ErrBadUsername), errors.Is(err, users.ErrBadPassword), errors.Is(err, users.ErrBadRole):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "error interno")
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
