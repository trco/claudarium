package handlers

import (
	"sort"

	"github.com/gofiber/fiber/v2"

	"github.com/trco/claudarium/internal/config"
)

func CapabilitiesPage(c *fiber.Ctx) error {
	caps := config.Capabilities()
	sources := map[string]bool{}
	for _, cap := range caps {
		sources[cap.Source] = true
	}
	return render(c, "capabilities", fiber.Map{
		"Nav": "capabilities", "Title": "Capabilities",
		"Caps": caps, "Sources": sortedKeys(sources),
	})
}

func PluginsPage(c *fiber.Ctx) error {
	plugins := config.Plugins()
	markets := map[string]bool{}
	for _, p := range plugins {
		if p.Marketplace != "" {
			markets[p.Marketplace] = true
		}
	}
	return render(c, "plugins", fiber.Map{
		"Nav": "plugins", "Title": "Plugins",
		"Plugins": plugins, "MarketplaceNames": sortedKeys(markets),
		"OK": c.Query("ok") != "",
	})
}

// PluginToggle enables/disables a plugin by writing settings.json (backed up).
func PluginToggle(c *fiber.Ctx) error {
	name := c.FormValue("name")
	enabled := c.FormValue("enabled") == "true"
	if err := config.SetPluginEnabled(name, enabled); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	verb := "Disabled"
	if enabled {
		verb = "Enabled"
	}
	config.AppendAudit("plugin", verb+" plugin "+name, "")
	return c.SendStatus(fiber.StatusNoContent)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func MarketplacesPage(c *fiber.Ctx) error {
	return render(c, "marketplaces", fiber.Map{
		"Nav": "marketplaces", "Title": "Marketplaces",
		"Marketplaces": config.Marketplaces(),
	})
}

func MCPPage(c *fiber.Ctx) error {
	return render(c, "mcp", fiber.Map{
		"Nav": "mcp", "Title": "MCP servers",
		"Servers": config.MCPServers(),
		"OK":      c.Query("ok") != "",
	})
}

// MCPToggle enables/disables an MCP server by writing ~/.claude.json (backed up).
func MCPToggle(c *fiber.Ctx) error {
	name := c.FormValue("name")
	scope := c.FormValue("scope")
	enabled := c.FormValue("enabled") == "true"
	if err := config.SetMCPEnabled(scope, name, enabled); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	state := "off"
	if enabled {
		state = "on"
	}
	config.AppendAudit("mcp", "Turned MCP "+name+" "+state+" ("+scope+")", "")
	return c.SendStatus(fiber.StatusNoContent)
}

func HealthPage(c *fiber.Ctx) error {
	return render(c, "health", fiber.Map{
		"Nav": "health", "Title": "Doctor",
		"Issues": config.HealthChecks(),
	})
}

func SearchPage(c *fiber.Ctx) error {
	q := c.Query("q")
	return render(c, "search", fiber.Map{
		"Nav": "search", "Title": "Search",
		"Query": q, "Results": config.Search(q),
	})
}

func SettingsPage(c *fiber.Ctx) error {
	return render(c, "settings", fiber.Map{
		"Nav": "settings", "Title": "Settings",
		"Settings": config.Settings(),
	})
}

func AuditPage(c *fiber.Ctx) error {
	return render(c, "audit", fiber.Map{
		"Nav": "audit", "Title": "Logs",
		"Entries": config.AuditEntries(),
	})
}

// AuditDelete drops one log entry (and its backup, if any), then reloads.
func AuditDelete(c *fiber.Ctx) error {
	if err := config.DeleteAuditEntry(c.FormValue("id")); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.Redirect("/audit")
}

// AuditClear wipes the whole log and its backups, then reloads.
func AuditClear(c *fiber.Ctx) error {
	if _, err := config.ClearAudit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.Redirect("/audit")
}
