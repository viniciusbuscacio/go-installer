package main

import (
	"strings"
	"testing"
)

func TestSetupReadmeOpensWizardWithoutApplicationsDrag(t *testing.T) {
	text := readmeText("Pop Desktop Setup.app", "setup")
	if !strings.Contains(text, "Double-click Pop Desktop Setup") {
		t.Fatalf("setup instructions do not name the wizard: %q", text)
	}
	if strings.Contains(text, "Drag") || strings.Contains(text, "/Applications") {
		t.Fatalf("setup instructions contain drag-to-Applications guidance: %q", text)
	}
}

func TestInstallReadmeKeepsApplicationsGuidance(t *testing.T) {
	text := readmeText("go-calc.app", "install")
	if !strings.Contains(text, "Drag go-calc onto the Applications folder") {
		t.Fatalf("install instructions lost Applications guidance: %q", text)
	}
}
