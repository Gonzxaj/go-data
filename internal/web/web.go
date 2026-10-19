// Package web renders the dashboard page from web/templates.
package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"

	"go-data/internal/domain"
)

// Section is one nav tab. Sections with a Key share the "view-resource"
// template (metrics + chart); the rest have their own view in views/.
type Section struct {
	Name, Icon, Key string
	AdminOnly       bool // hidden until the session turns out to be an admin
}

var Sections = []Section{
	{Name: "General", Icon: "activity"},
	{Name: "CPU", Icon: "cpu", Key: "cpu"},
	{Name: "Memoria", Icon: "memory", Key: "ram"},
	{Name: "Disco", Icon: "disk", Key: "disk"},
	{Name: "Red", Icon: "wifi", Key: "net"},
	{Name: "Conexiones", Icon: "link"},
	{Name: "Contenedores", Icon: "box"},
	{Name: "Seguridad", Icon: "shield"},
	{Name: "Usuarios", Icon: "users", AdminOnly: true},
}

type Page struct {
	Title    string
	CSS      string // stylesheet name under web/static/css, without .css
	Hostname string
	Sections []Section
}

// Register renders every page once at startup (server-side data is fixed
// then; live data comes from /api) and serves each one on its route.
func Register(mux *http.ServeMux, dir string, host domain.HostInfo) error {
	pages, err := Render(dir, host)
	if err != nil {
		return err
	}
	for route, page := range pages {
		mux.HandleFunc("GET "+route, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(page)
		})
	}
	return nil
}

// Render executes each page template, keyed by the route it is served on.
func Render(dir string, host domain.HostInfo) (map[string][]byte, error) {
	t := template.New("").Funcs(template.FuncMap{"dict": dict})
	for _, glob := range []string{"*.html", "components/*.html", "components/dialogs/*.html", "views/*.html"} {
		var err error
		if t, err = t.ParseGlob(filepath.Join(dir, glob)); err != nil {
			return nil, err
		}
	}
	pages := map[string]struct {
		tmpl string
		data Page
	}{
		"/{$}":        {"base.html", Page{Title: "GO-DATA | Estado del servidor", CSS: "app", Hostname: host.Hostname, Sections: Sections}},
		"/login.html": {"login.html", Page{Title: "GO-DATA | Iniciar sesión", CSS: "login"}},
	}
	out := make(map[string][]byte, len(pages))
	for route, p := range pages {
		var buf bytes.Buffer
		if err := t.ExecuteTemplate(&buf, p.tmpl, p.data); err != nil {
			return nil, err
		}
		out[route] = buf.Bytes()
	}
	return out, nil
}

// dict builds a map from key/value pairs so a template can pass several
// values to a sub-template: {{ template "x" (dict "A" 1 "B" 2) }}.
func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}
