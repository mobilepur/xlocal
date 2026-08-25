//go:build darwin

package keychain

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestKeychainRoundTripIntegration(t *testing.T) {
	if os.Getenv("XLOCAL_KEYCHAIN_INTEGRATION") != "1" {
		t.Skip("XLOCAL_KEYCHAIN_INTEGRATION not set")
	}

	suffix := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	name := "test-" + suffix
	kc := New()
	kc.Service = "xlocal-integration-" + suffix
	t.Cleanup(func() {
		_ = kc.Delete(name)
	})

	first := "dummy-secret-v1-" + suffix
	if err := kc.Set(name, first); err != nil {
		t.Fatalf("set first secret: %v", err)
	}
	if got, err := kc.Get(name); err != nil || got != first {
		t.Fatalf("get first secret = %q, %v", got, err)
	}

	updated := "dummy-secret-v2-" + suffix
	if err := kc.Set(name, updated); err != nil {
		t.Fatalf("update secret: %v", err)
	}
	if got, err := kc.Get(name); err != nil || got != updated {
		t.Fatalf("get updated secret = %q, %v", got, err)
	}

	if err := kc.Delete(name); err != nil {
		t.Fatalf("delete secret: %v", err)
	}
	if _, err := kc.Get(name); err == nil {
		t.Fatal("expected deleted secret to be absent")
	}
}
