package server

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
)

func (a *App) handleBankrollPage(c fiber.Ctx) error {
	balance, err := a.queries.GetBankrollBalanceCents(c.Context())
	if err != nil {
		return fmt.Errorf("get bankroll balance: %w", err)
	}

	entries, err := a.queries.ListBankrollEntries(c.Context(), 50)
	if err != nil {
		return fmt.Errorf("list bankroll entries: %w", err)
	}

	entryRows := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		entryRows = append(entryRows, map[string]any{
			"ID":            e.ID,
			"EntryType":     e.EntryType,
			"AmountDollars": formatCentsSigned(e.AmountCents),
			"AmountPositive": e.AmountCents >= 0,
			"ReferenceType": e.ReferenceType,
			"ReferenceID":   e.ReferenceID,
			"CreatedAt":     formatTimestamp(e.CreatedAt, "—"),
		})
	}

	_, overallStatus := a.pipelineView(c.Context(), sportFilterSelection{})

	view := map[string]any{
		"Title":              "Bankroll",
		"ActiveNav":          "bankroll",
		"OverallStatus":      overallStatus,
		"Environment":        a.cfg.Env,
		"Balance":            formatCents(balance),
		"BalanceCents":       balance,
		"Entries":            entryRows,
		"SelectedSportQuery": "",
	}

	return c.Render("pages/bankroll", view, "layouts/base")
}

func (a *App) handleBankrollDeposit(c fiber.Ctx) error {
	amountStr := strings.TrimSpace(c.FormValue("amount_dollars"))
	if amountStr == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Amount is required")
	}

	amountDollars, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amountDollars <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid deposit amount")
	}
	amountCents := int64(math.Round(amountDollars * 100))

	_, err = a.queries.InsertBankrollEntry(c.Context(), store.InsertBankrollEntryParams{
		EntryType:     "deposit",
		AmountCents:   amountCents,
		Currency:      "USD",
		ReferenceType: "manual",
		ReferenceID:   "deposit",
		Metadata:      json.RawMessage(`{}`),
	})
	if err != nil {
		return fmt.Errorf("insert deposit entry: %w", err)
	}

	return c.Redirect().To("/bankroll")
}
