package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/abundo/ergotime-server/internal/auth"
	"github.com/abundo/ergotime-server/internal/config"
	"github.com/abundo/ergotime-server/internal/database"
	"github.com/abundo/ergotime-server/internal/handlers"
	appmw "github.com/abundo/ergotime-server/internal/middleware"
	"github.com/abundo/ergotime-server/web"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (or set ERGOTIME_CONFIG)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	tokens := auth.NewTokenService(cfg)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// When behind a reverse HTTPS proxy, trust X-Forwarded-* headers
	e.IPExtractor = echo.ExtractIPFromXFFHeader()

	// Recover + request logging
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${time_rfc3339} ${id} ${remote_ip} ${method} ${uri} ${status} ${latency_human}\n",
	}))
	e.Use(appmw.SecurityHeaders(cfg))
	e.Use(appmw.CORS(cfg))
	// Body size limit (1 MiB) mitigates large payload DoS
	e.Use(middleware.BodyLimit("1M"))
	// Secure defaults for cookies / proxy
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "", // CSP supersedes X-XSS-Protection
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
		ContentSecurityPolicy: "", // set in SecurityHeaders for full control
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	}))

	// Custom error handler → JSON for API routes
	e.HTTPErrorHandler = jsonErrorHandler

	// Handlers
	authH := &handlers.AuthHandler{DB: db, Tokens: tokens, Cfg: cfg}
	usersH := &handlers.UsersHandler{DB: db}
	projectsH := &handlers.ProjectsHandler{DB: db}
	customersH := &handlers.CustomersHandler{DB: db}
	reportsH := &handlers.ReportsHandler{DB: db}
	timesheetsH := &handlers.TimesheetsHandler{DB: db}
	healthH := &handlers.HealthHandler{DB: db}

	loginLimiter := appmw.NewRateLimiter(cfg.Security.LoginRateLimit, time.Minute)

	// Health (no auth)
	e.GET("/healthz", healthH.Live)
	e.GET("/readyz", healthH.Ready)

	// Auth
	api := e.Group("/api")
	api.POST("/auth/login", authH.Login, appmw.LoginRateLimit(loginLimiter))
	api.POST("/auth/logout", authH.Logout)

	// Authenticated routes
	authMW := appmw.AuthRequired(tokens, cfg, db)
	adminMW := appmw.AdminRequired()

	api.GET("/auth/me", authH.Me, authMW)

	// Users — list/create/delete admin only; get/update self or admin
	api.GET("/users", usersH.List, authMW, adminMW)
	api.POST("/users", usersH.Create, authMW, adminMW)
	api.GET("/users/:id", usersH.Get, authMW)
	api.PUT("/users/:id", usersH.Update, authMW)
	api.DELETE("/users/:id", usersH.Delete, authMW, adminMW)

	// Customers
	api.GET("/customers", customersH.List, authMW)
	api.GET("/customers/:id", customersH.Get, authMW)
	api.POST("/customers", customersH.Create, authMW, adminMW)
	api.PUT("/customers/:id", customersH.Update, authMW, adminMW)
	api.DELETE("/customers/:id", customersH.Delete, authMW, adminMW)

	// Projects (under customers)
	api.GET("/projects", projectsH.List, authMW)
	api.GET("/projects/:id", projectsH.Get, authMW)
	api.POST("/projects", projectsH.Create, authMW, adminMW)
	api.PUT("/projects/:id", projectsH.Update, authMW, adminMW)
	api.DELETE("/projects/:id", projectsH.Delete, authMW, adminMW)

	// Reports
	api.GET("/reports", reportsH.List, authMW)
	api.GET("/reports/monthly", reportsH.Monthly, authMW)
	api.POST("/reports/batch", reportsH.Batch, authMW) // desktop batch push (before :id)
	api.GET("/reports/:id", reportsH.Get, authMW)
	api.POST("/reports", reportsH.Create, authMW)
	api.PUT("/reports/:id", reportsH.Update, authMW)
	api.DELETE("/reports/:id", reportsH.Delete, authMW)

	// Timesheets (PDF + invoice status + project reallocation per customer/month)
	api.GET("/timesheets", timesheetsH.List, authMW)
	api.GET("/timesheets/projects", timesheetsH.Projects, authMW)
	api.GET("/timesheets/pdf", timesheetsH.PDF, authMW)
	api.PUT("/timesheets/invoiced", timesheetsH.MarkInvoiced, authMW)
	api.PUT("/timesheets/reallocate", timesheetsH.Reallocate, authMW)

	// Legacy report paths (desktop client)
	api.GET("/report", reportsH.List, authMW)
	api.GET("/report/sync/:seq", reportsH.Sync, authMW)
	api.GET("/report/:id", reportsH.Get, authMW)
	api.POST("/report", reportsH.Create, authMW)
	api.PUT("/report/:id", reportsH.Update, authMW)
	api.DELETE("/report/:id", reportsH.Delete, authMW)

	// SPA: prefer optional disk dir (dev override); otherwise embedded assets.
	if fsys, source, ok := resolveStaticFS(cfg); ok {
		mountSPA(e, fsys, cfg.Static.SPA)
		log.Printf("static: serving SPA from %s (spa=%v)", source, cfg.Static.SPA)
	} else {
		log.Printf("static: SPA not mounted — build frontend (make frontend) and recompile")
	}

	addr := cfg.Addr()
	log.Printf("ergotime-server listening on %s (mode=%s)", addr, cfg.Server.Mode)
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// resolveStaticFS picks the SPA filesystem: on-disk static.dir if present, else embed.
func resolveStaticFS(cfg *config.Config) (fs.FS, string, bool) {
	if dir := strings.TrimSpace(cfg.Static.Dir); dir != "" {
		abs, err := filepath.Abs(dir)
		if err == nil {
			if st, err := os.Stat(abs); err == nil && st.IsDir() {
				if _, err := os.Stat(filepath.Join(abs, "index.html")); err == nil {
					return os.DirFS(abs), abs, true
				}
			}
		}
		log.Printf("static: dir %q not usable, falling back to embedded assets", dir)
	}

	fsys, err := web.DistFS()
	if err != nil {
		log.Printf("static: embed: %v", err)
		return nil, "", false
	}
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		return nil, "", false
	}
	return fsys, "embedded", true
}

func jsonErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	code := http.StatusInternalServerError
	msg := "internal server error"
	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		switch m := he.Message.(type) {
		case string:
			msg = m
		case error:
			msg = m.Error()
		default:
			msg = fmt.Sprint(m)
		}
	} else if cfgDebug(c) {
		msg = err.Error()
	}
	// Avoid leaking details for non-API
	_ = c.JSON(code, map[string]string{"message": msg})
}

func cfgDebug(c echo.Context) bool {
	return false
}

// mountSPA serves the Vue build from fsys and falls back to index.html for SPA routes.
func mountSPA(e *echo.Echo, fsys fs.FS, spa bool) {
	fileServer := http.FileServer(http.FS(fsys))

	serveIndex := func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-cache")
		b, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			return echo.ErrNotFound
		}
		return c.HTMLBlob(http.StatusOK, b)
	}

	e.GET("/", serveIndex)

	e.GET("/*", func(c echo.Context) error {
		reqPath := c.Request().URL.Path
		// Never hijack API or health routes (already registered, but belt-and-suspenders)
		if strings.HasPrefix(reqPath, "/api/") ||
			reqPath == "/healthz" || reqPath == "/readyz" {
			return echo.ErrNotFound
		}

		clean := path.Clean("/" + reqPath)
		name := strings.TrimPrefix(clean, "/")
		if name == "" || name == "." {
			return serveIndex(c)
		}

		// Open + stat via fs.FS (works for both embed and os.DirFS)
		f, err := fsys.Open(name)
		if err == nil {
			info, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !info.IsDir() {
				// Long-cache hashed Vite assets
				if strings.HasPrefix(clean, "/assets/") {
					c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(c.Response(), c.Request())
				return nil
			}
		}

		if spa {
			return serveIndex(c)
		}
		return echo.ErrNotFound
	})
}
