// Package server wires the Fiber app (routes, template engine, static files).
// Assets are embedded by default (self-contained binary); --dev reads them from
// disk so `air` can hot-reload templates and CSS.
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"

	assets "github.com/trco/claudarium"
	"github.com/trco/claudarium/internal/config"
	"github.com/trco/claudarium/internal/handlers"
)

func New(dev bool) *fiber.App {
	engine := templateEngine(dev)
	engine.AddFunc("lower", strings.ToLower)
	engine.AddFunc("hasPrefix", strings.HasPrefix)
	// readable splits compound ids like "plugin:foo@bar" into "plugin › foo › bar".
	readabler := strings.NewReplacer(":", " › ", "@", " › ")
	engine.AddFunc("readable", readabler.Replace)
	// sourceLabel shortens "repo:/a/b/c" to "repo:c" (folder name); others unchanged.
	engine.AddFunc("sourceLabel", func(s string) string {
		p, ok := strings.CutPrefix(s, "repo:")
		if !ok {
			return s
		}
		return "repo:" + p[strings.LastIndexByte(p, '/')+1:]
	})
	engine.AddFunc("json", func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	})

	app := fiber.New(fiber.Config{Views: engine, ViewsLayout: "layouts/base", PassLocalsToViews: true})
	app.Use(recover.New())
	app.Use(logger.New())
	mountStatic(app, dev)

	// Make nav badge counts available to every page render.
	app.Use(func(c *fiber.Ctx) error {
		if !strings.HasPrefix(c.Path(), "/static") {
			c.Locals("Counts", config.Counts())
		}
		return c.Next()
	})

	app.Get("/", func(c *fiber.Ctx) error { return c.Redirect("/memory") })

	app.Get("/memory", handlers.Memory)
	app.Post("/memory/render", handlers.MemoryRender)
	app.Post("/memory/preview", handlers.MemoryPreview)
	app.Post("/memory/apply", handlers.MemoryApply)

	app.Get("/capabilities", handlers.CapabilitiesPage)
	app.Get("/plugins", handlers.PluginsPage)
	app.Post("/plugins/toggle", handlers.PluginToggle)
	app.Get("/marketplaces", handlers.MarketplacesPage)
	app.Get("/mcp", handlers.MCPPage)
	app.Post("/mcp/toggle", handlers.MCPToggle)
	app.Get("/health", handlers.HealthPage)
	app.Get("/settings", handlers.SettingsPage)
	app.Get("/backups", handlers.BackupsPage)
	app.Post("/backups/delete", handlers.BackupDelete)
	app.Post("/backups/delete-all", handlers.BackupDeleteAll)
	app.Get("/search", handlers.SearchPage)
	app.Post("/open", handlers.OpenFile)
	app.Get("/raw", handlers.RawFile)

	return app
}

func templateEngine(dev bool) *html.Engine {
	if dev {
		e := html.New("web/templates", ".html")
		e.Reload(true)
		return e
	}
	sub, _ := fs.Sub(assets.FS, "web/templates")
	return html.NewFileSystem(http.FS(sub), ".html")
}

func mountStatic(app *fiber.App, dev bool) {
	if dev {
		app.Static("/static", "web/static")
		return
	}
	app.Use("/static", filesystem.New(filesystem.Config{
		Root:       http.FS(assets.FS),
		PathPrefix: "web/static",
	}))
}
