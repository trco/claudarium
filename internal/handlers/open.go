package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/trco/claudarium/internal/config"
)

// OpenFile reveals a capability's file in Finder.
func OpenFile(c *fiber.Ctx) error {
	if err := config.RevealFile(c.FormValue("path")); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RawFile returns the raw text of a capability file for in-app preview.
func RawFile(c *fiber.Ctx) error {
	b, err := config.ReadTextFile(c.Query("path"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	return c.Type("txt").Send(b)
}
