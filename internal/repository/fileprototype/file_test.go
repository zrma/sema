package fileprototype

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/repository/repositorytest"
)

const (
	crashHelperPath     = "SEMA_FILE_PROTOTYPE_HELPER_PATH"
	crashHelperPoint    = "SEMA_FILE_PROTOTYPE_HELPER_POINT"
	crashHelperExpected = "SEMA_FILE_PROTOTYPE_HELPER_EXPECTED"
	crashExitCode       = 91
)

func TestFilePrototypeRepositoryConformance(t *testing.T) {
	repositorytest.Run(t, func(t testing.TB) (repository.Repository, func() repository.Repository) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "repository.json")
		owner, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		return owner, func() repository.Repository {
			reopened, reopenErr := Open(path)
			if reopenErr != nil {
				t.Fatal(reopenErr)
			}
			return reopened
		}
	})
}

func TestFilePrototypeCrashBoundary(t *testing.T) {
	for _, point := range []faultPoint{faultAfterTempSync, faultAfterCommit} {
		t.Run(string(point), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "repository.json")
			owner, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			seeded, err := owner.Commit(context.Background(), crashOperation("seed", "seed"), []repository.Mutation{{
				Key: crashKey(), Payload: []byte("revision-1"),
			}})
			if err != nil {
				t.Fatal(err)
			}

			command := exec.Command(os.Args[0], "-test.run=^TestFilePrototypeCrashHelper$")
			command.Env = append(os.Environ(),
				crashHelperPath+"="+path,
				crashHelperPoint+"="+string(point),
				crashHelperExpected+"="+strconv.FormatUint(uint64(seeded.Version), 10),
			)
			err = command.Run()
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != crashExitCode {
				t.Fatalf("crash helper error = %v; want exit %d", err, crashExitCode)
			}

			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := reopened.Snapshot(context.Background(), "tenant-a")
			if err != nil {
				t.Fatal(err)
			}
			resource := findResource(t, snapshot, crashKey())
			retry, err := reopened.Commit(
				context.Background(),
				crashOperation("replace", "replace"),
				[]repository.Mutation{{
					Key: crashKey(), ExpectedVersion: seeded.Version, Payload: []byte("revision-2"),
				}},
			)
			if err != nil {
				t.Fatal(err)
			}
			switch point {
			case faultAfterTempSync:
				if string(resource.Payload) != "revision-1" || resource.Version != seeded.Version || retry.Replayed {
					t.Fatalf("pre-commit recovery resource=%#v retry=%#v", resource, retry)
				}
			case faultAfterCommit:
				if string(resource.Payload) != "revision-2" || resource.Version != retry.Version || !retry.Replayed {
					t.Fatalf("post-commit recovery resource=%#v retry=%#v", resource, retry)
				}
			}
		})
	}
}

func TestFilePrototypeCrashHelper(t *testing.T) {
	path := os.Getenv(crashHelperPath)
	if path == "" {
		return
	}
	point := faultPoint(os.Getenv(crashHelperPoint))
	expected, err := strconv.ParseUint(os.Getenv(crashHelperExpected), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := open(path, func(observed faultPoint) {
		if observed == point {
			os.Exit(crashExitCode)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = owner.Commit(context.Background(), crashOperation("replace", "replace"), []repository.Mutation{{
		Key: crashKey(), ExpectedVersion: repository.Version(expected), Payload: []byte("revision-2"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal("fault point was not reached")
}

func TestFilePrototypeRejectsCorruptionAndPublicMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repository.json")
	owner, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Commit(context.Background(), crashOperation("seed", "seed"), []repository.Mutation{{
		Key: crashKey(), Payload: []byte("active"),
	}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("repository mode = %04o; want 0600", info.Mode().Perm())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	corrupted := bytes.Replace(contents, []byte("replace_match_ticket"), []byte("replace_match_ticker"), 1)
	if bytes.Equal(corrupted, contents) {
		t.Fatal("fixture payload was not found")
	}
	if err := os.WriteFile(path, corrupted, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("corruption error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("public mode error = %v", err)
	}
}

func BenchmarkFilePrototypeRepository(b *testing.B) {
	repositorytest.BenchmarkAdapter(b, func(t testing.TB) (repository.Repository, func() repository.Repository) {
		path := filepath.Join(t.TempDir(), "repository.json")
		owner, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		return owner, func() repository.Repository {
			reopened, reopenErr := Open(path)
			if reopenErr != nil {
				t.Fatal(reopenErr)
			}
			return reopened
		}
	})
}

func crashOperation(id domain.OperationID, payload string) repository.Operation {
	return repository.Operation{
		Scope: "tenant-a", ID: id, Kind: "replace_match_ticket",
		Digest: repository.Digest([]byte(payload)),
		At:     time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
	}
}

func crashKey() repository.Key {
	return repository.Key{Scope: "tenant-a", Kind: "match_ticket", ID: "ticket-a"}
}

func findResource(
	t testing.TB,
	snapshot repository.Snapshot,
	key repository.Key,
) repository.Resource {
	t.Helper()
	for _, resource := range snapshot.Resources {
		if resource.Key == key {
			return resource
		}
	}
	t.Fatalf("resource %#v not found", key)
	return repository.Resource{}
}
