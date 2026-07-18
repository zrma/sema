package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zrma/sema/internal/repository"
)

func TestParseConfigurationRejectsUnsafeSchema(t *testing.T) {
	t.Setenv(connectionEnvironment, "postgres://example.invalid/database")
	var stderr bytes.Buffer
	_, err := parseConfiguration([]string{
		"-phase", "seed", "-schema", "unsafe-name", "-journal", "journal", "-manifest", "manifest",
	}, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "safe schema") {
		t.Fatalf("error = %v, stderr = %q", err, stderr.String())
	}
}

func TestParseConfigurationRollbackDoesNotNeedDatabase(t *testing.T) {
	var stderr bytes.Buffer
	configuration, err := parseConfiguration([]string{
		"-phase", "rollback", "-journal", "journal", "-manifest", "manifest",
	}, &stderr)
	if err != nil || configuration.schema != "" {
		t.Fatalf("configuration = %#v, error = %v, stderr = %q", configuration, err, stderr.String())
	}
}

func TestManifestRoundTripUsesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	expected := manifest{
		Schema: manifestSchema, SourceDigest: strings.Repeat("a", 64), SourceRecords: 3,
		SnapshotVersion: 4, ResourceCount: 5, ResourceDigest: strings.Repeat("b", 64),
		AuditCount: 4, AuditDigest: strings.Repeat("c", 64), AuthorityDigest: strings.Repeat("d", 64),
		TableRows: map[string]int{"table": 1},
	}
	if err := writeManifest(path, expected); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest mode = %o; want 600", info.Mode().Perm())
	}
	actual, err := readManifest(path)
	if err != nil || !manifestsEqual(expected, actual) {
		t.Fatalf("manifest = %#v, error = %v", actual, err)
	}
}

func TestResourceDigestCoversPayloadAndTombstone(t *testing.T) {
	left := repository.Snapshot{Version: 2, Resources: []repository.Resource{
		{Key: repository.Key{Scope: "tenant", Kind: "kind", ID: "id"}, Version: 1, Payload: []byte("left")},
	}}
	right := repository.CloneSnapshot(left)
	right.Resources[0].Payload = []byte("right")
	if resourceDigest(left) == resourceDigest(right) {
		t.Fatal("resource digest ignored payload")
	}
	right = repository.CloneSnapshot(left)
	right.Resources[0].Deleted = true
	right.Resources[0].Payload = nil
	if resourceDigest(left) == resourceDigest(right) {
		t.Fatal("resource digest ignored tombstone state")
	}
}

func TestRollbackReopensUnchangedV0Fixture(t *testing.T) {
	journalPath := filepath.Join(t.TempDir(), "legacy.journal")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	source, err := createLegacyFixture(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	evidence := manifest{
		Schema: manifestSchema, SourceDigest: source.Digest, SourceRecords: source.RecordCount,
		SnapshotVersion: 1, ResourceCount: 1, ResourceDigest: strings.Repeat("b", 64),
		AuditCount: 1, AuditDigest: strings.Repeat("c", 64), AuthorityDigest: strings.Repeat("d", 64),
		TableRows: map[string]int{"table": 1},
	}
	if err := writeManifest(manifestPath, evidence); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"-phase", "rollback", "-journal", journalPath, "-manifest", manifestPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	var actual report
	if err := json.Unmarshal(stdout.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Phase != "rollback" || actual.SourceRecords != source.RecordCount ||
		strings.Contains(stdout.String(), journalPath) {
		t.Fatalf("report = %#v, output = %q", actual, stdout.String())
	}
}
