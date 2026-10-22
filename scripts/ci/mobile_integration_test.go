package ci_test

import (
	"os"
	"strings"
	"testing"
)

func TestMobileIntegrationPullsThroughPullImage(t *testing.T) {
	b, err := os.ReadFile("mobile-integration.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	if strings.Contains(script, "compose pull") {
		t.Error("mobile-integration.sh pulls through compose, which has no mirror fallback")
	}
	if !strings.Contains(script, `pull-image.sh" "$image"`) {
		t.Error("mobile-integration.sh does not pull the compose images through pull-image.sh")
	}
	if !strings.Contains(script, "up -d --wait --pull never postgres nats") {
		t.Error("mobile-integration.sh lets compose pull on up")
	}
}
