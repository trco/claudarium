package handlers

import (
	"html/template"

	"github.com/gofiber/fiber/v2"

	"github.com/trco/claudarium/internal/config"
)

func Memory(c *fiber.Ctx) error {
	content, err := config.ReadMemory()
	if err != nil {
		return render(c, "memory", fiber.Map{"Nav": "memory", "Title": "Memory", "LoadError": err.Error()})
	}
	rendered, _ := config.RenderMarkdown(content)
	return render(c, "memory", fiber.Map{
		"Nav": "memory", "Title": "Memory",
		"Content": content, "Rendered": template.HTML(rendered),
		"Path": config.MemoryPath(),
		"OK":   c.Query("ok") != "",
	})
}

// MemoryRender returns the live markdown preview for the editor's content.
func MemoryRender(c *fiber.Ctx) error {
	out, err := config.RenderMarkdown(c.FormValue("content"))
	if err != nil {
		return c.Type("html").SendString("<p class='text-red-600'>" + err.Error() + "</p>")
	}
	return c.Type("html").SendString(out)
}

func MemoryPreview(c *fiber.Ctx) error {
	orig, _ := config.ReadMemory()
	next := c.FormValue("content")
	d := config.LineDiff(orig, next)
	return c.Render("partials/diff", fiber.Map{
		"Diff": d, "HasChanges": config.HasChanges(d),
		"ApplyURL": "/memory/apply", "Include": "#memory-form",
	})
}

func MemoryApply(c *fiber.Ctx) error {
	if _, err := config.SaveFile(config.MemoryPath(), []byte(c.FormValue("content"))); err != nil {
		return applyError(c, err)
	}
	c.Set("HX-Redirect", "/memory?ok=1")
	return c.SendStatus(fiber.StatusOK)
}
