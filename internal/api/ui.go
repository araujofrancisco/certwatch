package api

import (
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

//go:embed web/templates/*.html
var templateFS embed.FS

//go:embed web/static
var staticFS embed.FS

type pageData struct {
	Title  string
	Active string
}

type pageTmpl struct {
	tmpl   *template.Template
	layout string
}

var (
	pageTemplates map[string]*pageTmpl
	loadTemplates sync.Once
)

func ensureTemplates() error {
	var err error
	loadTemplates.Do(func() {
		pageTemplates = make(map[string]*pageTmpl)
		for _, p := range []string{"dashboard", "domains", "domain-detail", "certificates", "reports", "import", "settings"} {
			t, e := template.ParseFS(templateFS,
				"web/templates/layout.html",
				"web/templates/"+p+".html",
			)
			if e != nil {
				err = e
				return
			}
			pageTemplates[p] = &pageTmpl{tmpl: t, layout: "layout.html"}
		}
		for _, p := range []string{"login", "register"} {
			t, e := template.ParseFS(templateFS,
				"web/templates/auth-layout.html",
				"web/templates/"+p+".html",
			)
			if e != nil {
				err = e
				return
			}
			pageTemplates[p] = &pageTmpl{tmpl: t, layout: "auth-layout.html"}
		}
	})
	return err
}

func (h *Handler) RegisterUIRoutes(mux *http.ServeMux) {
	if err := ensureTemplates(); err != nil {
		slog.Error("failed to load templates", "error", err)
	}

	staticSub, _ := fs.Sub(staticFS, "web/static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	})

	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "login", pageData{Title: "Login"})
	})

	mux.HandleFunc("GET /register", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "register", pageData{Title: "Register"})
	})

	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "dashboard", pageData{Title: "Dashboard", Active: "dashboard"})
	})

	mux.HandleFunc("GET /domains", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "domains", pageData{Title: "Domains", Active: "domains"})
	})

	// Static shell page like the other UI routes; the domain data is fetched
	// client-side via the JSON API, which handles auth and login redirects.
	mux.HandleFunc("GET /domains/", func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.Split(strings.TrimPrefix(r.URL.Path, "/domains/"), "/")[0]
		if _, err := strconv.ParseInt(idStr, 10, 64); err != nil {
			http.NotFound(w, r)
			return
		}
		renderPage(w, "domain-detail", pageData{
			Title:  "Domain Detail",
			Active: "domains",
		})
	})

	mux.HandleFunc("GET /certificates", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "certificates", pageData{Title: "Certificates", Active: "certificates"})
	})

	mux.HandleFunc("GET /reports", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "reports", pageData{Title: "Reports", Active: "reports"})
	})

	mux.HandleFunc("GET /import", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "import", pageData{Title: "Import Domains", Active: "import"})
	})
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, "settings", pageData{Title: "Settings", Active: "settings"})
	})
}

func renderPage(w http.ResponseWriter, name string, data pageData) {
	pt, ok := pageTemplates[name]
	if !ok {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pt.tmpl.ExecuteTemplate(w, pt.layout, data); err != nil {
		slog.Error("template execution failed", "page", name, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}
