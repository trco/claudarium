// Package handlers holds the Fiber HTTP handlers for the config UI.
package handlers

import "github.com/gofiber/fiber/v2"

// render renders a page with the base layout.
func render(c *fiber.Ctx, view string, data fiber.Map) error {
	return c.Render(view, data, "layouts/base")
}

// applyError returns the error to HTMX as a visible message (no redirect).
func applyError(c *fiber.Ctx, err error) error {
	c.Set("HX-Retarget", "#modal")
	c.Set("HX-Reswap", "innerHTML")
	return c.Status(fiber.StatusOK).Render("partials/diff", fiber.Map{"Error": err.Error()})
}
