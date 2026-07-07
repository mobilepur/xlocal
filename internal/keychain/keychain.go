// Package keychain stores API keys in the macOS keychain via the security
// CLI, so secrets never live in plain files.
package keychain

import (
	"fmt"
	"os/exec"
	"strings"
)

// SecretStore abstracts secret storage so flows can be tested with an
// in-memory fake.
type SecretStore interface {
	Set(name, secret string) error
	Get(name string) (string, error)
	Delete(name string) error
}

// Service is the keychain service name all xlocal secrets are filed under.
const Service = "xlocal"

type Keychain struct {
	Service string
	// runner executes the security CLI; replaced in tests.
	runner func(args ...string) (string, error)
}

func New() *Keychain {
	return &Keychain{Service: Service, runner: runSecurity}
}

func runSecurity(args ...string) (string, error) {
	out, err := exec.Command("security", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("security %s: %w", args[0], err)
	}
	return string(out), nil
}

// Set stores (or updates, thanks to -U) a secret under the given name.
func (k *Keychain) Set(name, secret string) error {
	_, err := k.runner("add-generic-password", "-a", name, "-s", k.Service, "-w", secret, "-U")
	if err != nil {
		return fmt.Errorf("storing key %q in keychain: %w", name, err)
	}
	return nil
}

func (k *Keychain) Get(name string) (string, error) {
	out, err := k.runner("find-generic-password", "-a", name, "-s", k.Service, "-w")
	if err != nil {
		return "", fmt.Errorf("key %q not found in keychain: %w", name, err)
	}
	return strings.TrimSpace(out), nil
}

func (k *Keychain) Delete(name string) error {
	_, err := k.runner("delete-generic-password", "-a", name, "-s", k.Service)
	if err != nil {
		return fmt.Errorf("deleting key %q from keychain: %w", name, err)
	}
	return nil
}

// InMemory is a SecretStore for tests.
type InMemory struct {
	secrets map[string]string
}

func NewInMemory() *InMemory {
	return &InMemory{secrets: make(map[string]string)}
}

func (m *InMemory) Set(name, secret string) error {
	m.secrets[name] = secret
	return nil
}

func (m *InMemory) Get(name string) (string, error) {
	secret, ok := m.secrets[name]
	if !ok {
		return "", fmt.Errorf("key %q not found", name)
	}
	return secret, nil
}

func (m *InMemory) Delete(name string) error {
	delete(m.secrets, name)
	return nil
}
