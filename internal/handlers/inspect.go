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
	})
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
	})
}

func HealthPage(c *fiber.Ctx) error {
	return render(c, "health", fiber.Map{
		"Nav": "health", "Title": "Doctor",
		"Issues": config.HealthChecks(),
	})
}
