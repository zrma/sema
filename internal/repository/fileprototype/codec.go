package fileprototype

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const fileSchema = "sema-repository-file-prototype-v1"

type diskEnvelope struct {
	Schema   string    `json:"schema"`
	State    diskState `json:"state"`
	Checksum string    `json:"checksum"`
}

type diskState struct {
	Version       repository.Version            `json:"version"`
	ScopeVersions map[string]repository.Version `json:"scope_versions"`
	Resources     []repository.Resource         `json:"resources"`
	Operations    []diskOperation               `json:"operations"`
	Audit         []diskAudit                   `json:"audit"`
}

type diskOperation struct {
	Scope   string             `json:"scope"`
	ID      domain.OperationID `json:"id"`
	Digest  string             `json:"digest"`
	Version repository.Version `json:"version"`
}

type diskAudit struct {
	Scope  string                 `json:"scope"`
	Record repository.AuditRecord `json:"record"`
}

func persistState(path string, current state, fault faultFunc) error {
	disk := stateToDisk(current)
	checksum, err := stateChecksum(disk)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(diskEnvelope{Schema: fileSchema, State: disk, Checksum: checksum})
	if err != nil {
		return fmt.Errorf("encode repository prototype: %w", err)
	}
	encoded = append(encoded, '\n')

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create repository directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".sema-repository-*")
	if err != nil {
		return fmt.Errorf("create repository temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set repository temporary mode: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		return fmt.Errorf("write repository temporary state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync repository temporary state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close repository temporary state: %w", err)
	}
	if fault != nil {
		fault(faultAfterTempSync)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace repository state: %w", err)
	}
	removeTemporary = false
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open repository directory: %w", err)
	}
	if err := directoryHandle.Sync(); err != nil {
		_ = directoryHandle.Close()
		return fmt.Errorf("sync repository directory: %w", err)
	}
	if err := directoryHandle.Close(); err != nil {
		return fmt.Errorf("close repository directory: %w", err)
	}
	if fault != nil {
		fault(faultAfterCommit)
	}
	return nil
}

func loadState(path string) (state, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newState(), nil
		}
		return state{}, fmt.Errorf("open repository prototype: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return state{}, fmt.Errorf("stat repository prototype: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return state{}, fmt.Errorf("repository prototype permissions are %04o; want private mode", info.Mode().Perm())
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return state{}, fmt.Errorf("read repository prototype: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var envelope diskEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return state{}, fmt.Errorf("decode repository prototype: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return state{}, fmt.Errorf("decode repository prototype trailing content")
	}
	if envelope.Schema != fileSchema {
		return state{}, fmt.Errorf("repository prototype schema is %q; want %q", envelope.Schema, fileSchema)
	}
	wantChecksum, err := stateChecksum(envelope.State)
	if err != nil {
		return state{}, err
	}
	if envelope.Checksum != wantChecksum {
		return state{}, fmt.Errorf("repository prototype checksum mismatch")
	}
	return diskToState(envelope.State)
}

func stateToDisk(current state) diskState {
	disk := diskState{
		Version: current.version, ScopeVersions: make(map[string]repository.Version, len(current.scopeVersions)),
		Resources:  make([]repository.Resource, 0, len(current.resources)),
		Operations: make([]diskOperation, 0, len(current.operations)),
		Audit:      make([]diskAudit, len(current.audit)),
	}
	for scope, version := range current.scopeVersions {
		disk.ScopeVersions[scope] = version
	}
	for _, resource := range current.resources {
		resource.Payload = slices.Clone(resource.Payload)
		disk.Resources = append(disk.Resources, resource)
	}
	slices.SortFunc(disk.Resources, func(left, right repository.Resource) int {
		return compareKey(left.Key, right.Key)
	})
	for key, receipt := range current.operations {
		disk.Operations = append(disk.Operations, diskOperation{
			Scope: key.scope, ID: key.id, Digest: hex.EncodeToString(receipt.digest[:]), Version: receipt.result.Version,
		})
	}
	slices.SortFunc(disk.Operations, func(left, right diskOperation) int {
		if left.Scope != right.Scope {
			if left.Scope < right.Scope {
				return -1
			}
			return 1
		}
		if left.ID < right.ID {
			return -1
		}
		if left.ID > right.ID {
			return 1
		}
		return 0
	})
	for index, scoped := range current.audit {
		disk.Audit[index] = diskAudit{
			Scope: scoped.scope, Record: repository.CloneAudit([]repository.AuditRecord{scoped.record})[0],
		}
	}
	return disk
}

func diskToState(disk diskState) (state, error) {
	loaded := newState()
	loaded.version = disk.Version
	for scope, version := range disk.ScopeVersions {
		if scope == "" || version == 0 || version > disk.Version {
			return state{}, fmt.Errorf("repository prototype has invalid scope version")
		}
		loaded.scopeVersions[scope] = version
	}
	for _, resource := range disk.Resources {
		if resource.Key.Scope == "" || resource.Key.Kind == "" || resource.Key.ID == "" ||
			resource.Version == 0 || resource.Version > disk.Version ||
			loaded.scopeVersions[resource.Key.Scope] < resource.Version {
			return state{}, fmt.Errorf("repository prototype has invalid resource metadata")
		}
		if _, exists := loaded.resources[resource.Key]; exists {
			return state{}, fmt.Errorf("repository prototype repeats a resource")
		}
		resource.Payload = slices.Clone(resource.Payload)
		loaded.resources[resource.Key] = resource
	}
	for _, operation := range disk.Operations {
		if operation.Scope == "" || operation.ID == "" || operation.Version == 0 ||
			operation.Version > disk.Version || loaded.scopeVersions[operation.Scope] < operation.Version {
			return state{}, fmt.Errorf("repository prototype has invalid operation metadata")
		}
		digestBytes, err := hex.DecodeString(operation.Digest)
		if err != nil || len(digestBytes) != sha256.Size {
			return state{}, fmt.Errorf("repository prototype has invalid operation digest")
		}
		var digest [sha256.Size]byte
		copy(digest[:], digestBytes)
		key := operationKey{scope: operation.Scope, id: operation.ID}
		if _, exists := loaded.operations[key]; exists {
			return state{}, fmt.Errorf("repository prototype repeats an operation")
		}
		loaded.operations[key] = operationReceipt{
			digest: digest, result: repository.CommitResult{Version: operation.Version},
		}
	}
	lastAuditVersion := repository.Version(0)
	for _, audit := range disk.Audit {
		if audit.Scope == "" || audit.Record.Version <= lastAuditVersion ||
			audit.Record.Version > disk.Version || loaded.scopeVersions[audit.Scope] < audit.Record.Version ||
			audit.Record.OperationKind == "" || audit.Record.At.IsZero() || len(audit.Record.ResourceCounts) == 0 {
			return state{}, fmt.Errorf("repository prototype has invalid audit metadata")
		}
		for kind, count := range audit.Record.ResourceCounts {
			if kind == "" || count <= 0 {
				return state{}, fmt.Errorf("repository prototype has invalid audit resource count")
			}
		}
		lastAuditVersion = audit.Record.Version
		loaded.audit = append(loaded.audit, scopedAuditRecord{
			scope: audit.Scope, record: repository.CloneAudit([]repository.AuditRecord{audit.Record})[0],
		})
	}
	if len(loaded.operations) != len(loaded.audit) {
		return state{}, fmt.Errorf("repository prototype operation and audit receipt counts differ")
	}
	return loaded, nil
}

func stateChecksum(disk diskState) (string, error) {
	encoded, err := json.Marshal(disk)
	if err != nil {
		return "", fmt.Errorf("encode repository prototype checksum: %w", err)
	}
	digest := sha256.Sum256(append([]byte(fileSchema+"\n"), encoded...))
	return hex.EncodeToString(digest[:]), nil
}
