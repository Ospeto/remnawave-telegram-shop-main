package handler

import (
	"strings"
	"testing"
	"time"

	hc "remnawave-tg-shop-bot/internal/service/healthcheck"
)

func TestRenderAdminScreenIncludesHealthcheckAction(t *testing.T) {
	text, keyboard := Handler{}.renderAdminScreen(nil, screenAdminOperations)
	if !strings.Contains(text, "Operations") {
		t.Fatalf("renderAdminScreen() text = %q, want operations copy", text)
	}

	found := false
	for _, row := range keyboard {
		for _, button := range row {
			if button.Text == "Run E2E Check" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("renderAdminScreen() missing Run E2E Check button")
	}
}

func TestRenderAdminHomeIncludesPromosSection(t *testing.T) {
	text, keyboard := Handler{}.renderAdminScreen(nil, screenAdminHome)
	if !strings.Contains(text, "Dashboard") {
		t.Fatalf("renderAdminScreen() text = %q, want dashboard copy", text)
	}

	found := false
	for _, row := range keyboard {
		for _, button := range row {
			if button.Text == "Promos" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("renderAdminScreen() missing Promos button on home screen")
	}
}

func TestFormatHealthcheckReport(t *testing.T) {
	report := &hc.Report{
		Success:  true,
		Duration: 2 * time.Second,
		Steps: []hc.StepResult{
			{Name: "Analyzer", Status: hc.StepPass, Detail: "openrouter ok"},
			{Name: "Workflow", Status: hc.StepPass, Detail: "synthetic purchase fulfilled"},
		},
	}

	text := formatHealthcheckReport(report)
	if !strings.Contains(text, "E2E Healthcheck: PASS") {
		t.Fatalf("formatHealthcheckReport() = %q, want PASS header", text)
	}
	if !strings.Contains(text, "Analyzer") || !strings.Contains(text, "Workflow") {
		t.Fatalf("formatHealthcheckReport() = %q, want step names", text)
	}
}
