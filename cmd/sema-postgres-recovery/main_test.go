package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseConfigurationRejectsUnsafeOrProductionShapedInput(t *testing.T) {
	t.Setenv(postgresTestDSNEnvironment, "postgres://example.invalid/sema")
	for name, args := range map[string][]string{
		"unsafe schema": {
			"-phase", "seed", "-schema", "unsafe-name", "-manifest", "manifest.json",
		},
		"missing manifest": {
			"-phase", "verify", "-schema", "sema_recovery",
		},
		"unknown phase": {
			"-phase", "restore", "-schema", "sema_recovery", "-manifest", "manifest.json",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			if _, err := parseConfiguration(args, &stderr); err == nil || stderr.Len() == 0 {
				t.Fatalf("unsafe configuration was accepted: %v", args)
			}
		})
	}
}

func TestParseConfigurationRequiresTestDatabaseEnvironment(t *testing.T) {
	t.Setenv(postgresTestDSNEnvironment, "")
	var stderr bytes.Buffer
	_, err := parseConfiguration([]string{
		"-phase", "seed", "-schema", "sema_recovery", "-manifest", "manifest.json",
	}, &stderr)
	if err == nil || !strings.Contains(stderr.String(), postgresTestDSNEnvironment) {
		t.Fatalf("error=%v stderr=%q", err, stderr.String())
	}
}
