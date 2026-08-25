// Package keychain stores API keys in the macOS keychain via the security
// CLI, so secrets never live in plain files.
package keychain

import (
	"fmt"
	"io"
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

const securityPath = "/usr/bin/security"

type Keychain struct {
	Service string
	// runner executes the security CLI; replaced in tests.
	runner func(path string, stdin io.Reader, args ...string) (string, error)
}

func New() *Keychain {
	return &Keychain{Service: Service, runner: runCommand}
}

func runCommand(path string, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.Command(path, args...)
	cmd.Stdin = stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		command := path
		if len(args) > 0 {
			command += " " + args[0]
		}
		return "", fmt.Errorf("%s: %w", command, err)
	}
	return string(out), nil
}

// Set stores (or updates, thanks to -U) a secret under the given name.
func (k *Keychain) Set(name, secret string) error {
	// With -w as the final option, security prompts for the password and its
	// confirmation. Supplying both over stdin keeps them out of process args.
	_, err := k.runner(
		securityPath,
		strings.NewReader(secret+"\n"+secret+"\n"),
		"add-generic-password", "-a", name, "-s", k.Service, "-U", "-w",
	)
	if err != nil {
		return fmt.Errorf("storing key %q in keychain: %w", name, err)
	}
	return nil
}

func (k *Keychain) Get(name string) (string, error) {
	out, err := k.runner(securityPath, nil, "find-generic-password", "-a", name, "-s", k.Service, "-w")
	if err != nil {
		return "", fmt.Errorf("key %q not found in keychain: %w", name, err)
	}
	return strings.TrimSpace(out), nil
}

func (k *Keychain) Delete(name string) error {
	_, err := k.runner(securityPath, nil, "delete-generic-password", "-a", name, "-s", k.Service)
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
