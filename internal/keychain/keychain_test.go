package keychain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeRunner records security invocations and plays back canned results.
type fakeRunner struct {
	calls  [][]string
	output string
	err    error
}

func (f *fakeRunner) run(args ...string) (string, error) {
	f.calls = append(f.calls, args)
	return f.output, f.err
}

func TestSetInvokesSecurityAddGenericPassword(t *testing.T) {
	f := &fakeRunner{}
	kc := &Keychain{Service: "xlocal", runner: f.run}

	if err := kc.Set("work", "sk-ant-123"); err != nil {
		t.Fatal(err)
	}

	if len(f.calls) != 1 {
		t.Fatalf("expected 1 security call, got %d", len(f.calls))
	}
	want := []string{"add-generic-password", "-a", "work", "-s", "xlocal", "-w", "sk-ant-123", "-U"}
	if !reflect.DeepEqual(f.calls[0], want) {
		t.Errorf("args = %v, want %v", f.calls[0], want)
	}
}

func TestGetReturnsTrimmedSecret(t *testing.T) {
	f := &fakeRunner{output: "sk-ant-123\n"}
	kc := &Keychain{Service: "xlocal", runner: f.run}

	secret, err := kc.Get("work")
	if err != nil {
		t.Fatal(err)
	}
	if secret != "sk-ant-123" {
		t.Errorf("secret = %q", secret)
	}

	want := []string{"find-generic-password", "-a", "work", "-s", "xlocal", "-w"}
	if !reflect.DeepEqual(f.calls[0], want) {
		t.Errorf("args = %v, want %v", f.calls[0], want)
	}
}

func TestGetWrapsNotFound(t *testing.T) {
	f := &fakeRunner{err: errors.New("exit status 44")}
	kc := &Keychain{Service: "xlocal", runner: f.run}

	_, err := kc.Get("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should name the key: %v", err)
	}
}

func TestDeleteInvokesSecurityDelete(t *testing.T) {
	f := &fakeRunner{}
	kc := &Keychain{Service: "xlocal", runner: f.run}

	if err := kc.Delete("work"); err != nil {
		t.Fatal(err)
	}
	want := []string{"delete-generic-password", "-a", "work", "-s", "xlocal"}
	if !reflect.DeepEqual(f.calls[0], want) {
		t.Errorf("args = %v, want %v", f.calls[0], want)
	}
}

func TestInMemoryStore(t *testing.T) {
	var store SecretStore = NewInMemory()

	if _, err := store.Get("nope"); err == nil {
		t.Error("expected error for unknown key")
	}

	if err := store.Set("a", "secret"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("a")
	if err != nil || got != "secret" {
		t.Errorf("Get = %q, %v", got, err)
	}

	if err := store.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("a"); err == nil {
		t.Error("expected error after delete")
	}
}
