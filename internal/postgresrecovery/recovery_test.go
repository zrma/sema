package postgresrecovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateManifestRoundTripAndReportRedaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	expected := manifest{
		Schema: ManifestSchema, SnapshotVersion: 12, ResourceCount: 20,
		ResourceDigest: strings.Repeat("a", 64), AuditCount: 12,
		AuditDigest: strings.Repeat("b", 64), AuthorityDigest: strings.Repeat("c", 64),
		TableRows: map[string]int{"sema_repository_resources": 20},
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
		t.Fatalf("manifest=%#v error=%v", actual, err)
	}
	report := reportFor("verify", actual, true)
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		path, recoveryScope, checkpointPolicy, checkpointRun, recoveryToken,
		expected.ResourceDigest, expected.AuthorityDigest,
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("report exposed private recovery evidence %q: %s", forbidden, encoded)
		}
	}
}

func TestManifestDigestDetectsSemanticChanges(t *testing.T) {
	left := manifest{
		Schema: ManifestSchema, SnapshotVersion: 1, ResourceCount: 1,
		ResourceDigest: strings.Repeat("a", 64), AuditCount: 1,
		AuditDigest: strings.Repeat("b", 64), AuthorityDigest: strings.Repeat("c", 64),
		TableRows: map[string]int{"sema_repository_resources": 1},
	}
	right := left
	right.TableRows = map[string]int{"sema_repository_resources": 2}
	if manifestsEqual(left, right) {
		t.Fatal("semantic manifest equality ignored table row changes")
	}
}
