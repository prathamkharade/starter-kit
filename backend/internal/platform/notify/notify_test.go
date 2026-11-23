package notify

import (
	"strings"
	"testing"
)

func TestRenderOTP(t *testing.T) {
	e, err := Render("Acme", "otp", map[string]any{"Code": "123456", "Minutes": 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.Subject, "123456") || !strings.Contains(e.HTML, "Acme") || !strings.Contains(e.Text, "10 minutes") {
		t.Fatalf("bad render: %+v", e)
	}
}

func TestRenderEscapesHTML(t *testing.T) {
	e, _ := Render("<script>", "otp", map[string]any{"Code": "1", "Minutes": 1})
	if strings.Contains(e.HTML, "<script>") {
		t.Fatal("html not escaped")
	}
}
