package keychain

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type runnerCall struct {
	path  string
	stdin string
	args  []string
}

// fakeRunner records security invocations and plays back canned results.
type fakeRunner struct {
	calls  []runnerCall
	output string
	err    error
}

func (f *fakeRunner) run(path string, stdin io.Reader, args ...string) (string, error) {
	var input []byte
	if stdin != nil {
		var err error
		input, err = io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
	}
	f.calls = append(f.calls, runnerCall{
		path:  path,
		stdin: string(input),
		args:  append([]string(nil), args...),
	})
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
	call := f.calls[0]
	if call.path != securityPath {
		t.Errorf("path = %q, want %q", call.path, securityPath)
	}
	if call.stdin != "sk-ant-123\nsk-ant-123\n" {
		t.Errorf("stdin = %q, want secret and confirmation on separate lines", call.stdin)
	}
	want := []string{"add-generic-password", "-a", "work", "-s", "xlocal", "-U", "-w"}
	if !reflect.DeepEqual(call.args, want) {
		t.Errorf("args = %v, want %v", call.args, want)
	}
	if strings.Contains(strings.Join(call.args, " "), "sk-ant-123") {
		t.Error("secret must not be passed in process arguments")
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

	call := f.calls[0]
	if call.path != securityPath {
		t.Errorf("path = %q, want %q", call.path, securityPath)
	}
	if call.stdin != "" {
		t.Errorf("stdin = %q, want empty", call.stdin)
	}
	want := []string{"find-generic-password", "-a", "work", "-s", "xlocal", "-w"}
	if !reflect.DeepEqual(call.args, want) {
		t.Errorf("args = %v, want %v", call.args, want)
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
	call := f.calls[0]
	if call.path != securityPath {
		t.Errorf("path = %q, want %q", call.path, securityPath)
	}
	if call.stdin != "" {
		t.Errorf("stdin = %q, want empty", call.stdin)
	}
	want := []string{"delete-generic-password", "-a", "work", "-s", "xlocal"}
	if !reflect.DeepEqual(call.args, want) {
		t.Errorf("args = %v, want %v", call.args, want)
	}
}

func TestRunCommandDoesNotLeakStdinInError(t *testing.T) {
	const secret = "sk-ant-must-not-leak"
	_, err := runCommand("/usr/bin/false", strings.NewReader(secret), "ignored")
	if err == nil {
		t.Fatal("expected command error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error leaked stdin: %v", err)
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
