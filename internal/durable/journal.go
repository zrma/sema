package durable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const journalSchema = "sema-journal-v1"

// Record is one checksummed, monotonically sequenced durable decision.
type Record struct {
	Schema   string          `json:"schema"`
	Sequence uint64          `json:"sequence"`
	Kind     string          `json:"kind"`
	Payload  json.RawMessage `json:"payload"`
	Checksum string          `json:"checksum"`
}

type unsignedRecord struct {
	Schema   string          `json:"schema"`
	Sequence uint64          `json:"sequence"`
	Kind     string          `json:"kind"`
	Payload  json.RawMessage `json:"payload"`
}

type journal struct {
	file    *os.File
	records []Record
	closed  bool
}

func openJournal(path string) (*journal, error) {
	if path == "" {
		return nil, fmt.Errorf("journal path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create journal directory: %w", err)
	}
	_, statErr := os.Stat(path)
	created := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !created {
		return nil, fmt.Errorf("stat journal path: %w", statErr)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	closeOnError := func(openErr error) (*journal, error) {
		_ = file.Close()
		return nil, openErr
	}
	if err := file.Chmod(0o600); err != nil {
		return closeOnError(fmt.Errorf("restrict journal permissions: %w", err))
	}
	if created {
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return closeOnError(fmt.Errorf("sync journal directory: %w", err))
		}
	}
	if err := lockFile(file); err != nil {
		return closeOnError(fmt.Errorf("lock journal: %w", err))
	}
	records, err := recoverRecords(file)
	if err != nil {
		_ = unlockFile(file)
		return closeOnError(err)
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = unlockFile(file)
		return closeOnError(fmt.Errorf("seek journal end: %w", err))
	}
	return &journal{file: file, records: records}, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (journal *journal) append(kind eventKind, payload any) (Record, error) {
	if journal.closed {
		return Record{}, fmt.Errorf("journal is closed")
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Record{}, fmt.Errorf("encode durable payload: %w", err)
	}
	record, line, err := encodeRecord(uint64(len(journal.records)+1), string(kind), payloadJSON)
	if err != nil {
		return Record{}, err
	}
	stat, err := journal.file.Stat()
	if err != nil {
		return Record{}, fmt.Errorf("stat journal: %w", err)
	}
	if _, err := journal.file.Seek(0, io.SeekEnd); err != nil {
		return Record{}, fmt.Errorf("seek journal end: %w", err)
	}
	line = append(line, '\n')
	if err := writeFull(journal.file, line); err != nil {
		return Record{}, journal.rollbackAppend(stat.Size(), fmt.Errorf("append journal: %w", err))
	}
	if err := journal.file.Sync(); err != nil {
		return Record{}, journal.rollbackAppend(stat.Size(), fmt.Errorf("sync journal: %w", err))
	}
	journal.records = append(journal.records, cloneRecord(record))
	return cloneRecord(record), nil
}

func (journal *journal) rollbackAppend(size int64, appendErr error) error {
	if err := journal.file.Truncate(size); err != nil {
		return errors.Join(appendErr, fmt.Errorf("truncate failed append: %w", err))
	}
	if _, err := journal.file.Seek(0, io.SeekEnd); err != nil {
		return errors.Join(appendErr, fmt.Errorf("seek after append rollback: %w", err))
	}
	if err := journal.file.Sync(); err != nil {
		return errors.Join(appendErr, fmt.Errorf("sync append rollback: %w", err))
	}
	return appendErr
}

func (journal *journal) Records() []Record {
	return cloneRecords(journal.records)
}

func (journal *journal) reload() ([]Record, error) {
	if journal.closed {
		return nil, fmt.Errorf("journal is closed")
	}
	records, err := recoverRecords(journal.file)
	if err != nil {
		return nil, err
	}
	if _, err := journal.file.Seek(0, io.SeekEnd); err != nil {
		return nil, fmt.Errorf("seek journal end: %w", err)
	}
	journal.records = records
	return cloneRecords(records), nil
}

func (journal *journal) Close() error {
	if journal.closed {
		return nil
	}
	journal.closed = true
	unlockErr := unlockFile(journal.file)
	closeErr := journal.file.Close()
	return errors.Join(unlockErr, closeErr)
}

func recoverRecords(file *os.File) ([]Record, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek journal start: %w", err)
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}
	complete := contents
	if len(contents) > 0 && contents[len(contents)-1] != '\n' {
		lastNewline := bytes.LastIndexByte(contents, '\n')
		completeSize := lastNewline + 1
		complete = contents[:completeSize]
		if err := file.Truncate(int64(completeSize)); err != nil {
			return nil, fmt.Errorf("truncate incomplete journal tail: %w", err)
		}
		if err := file.Sync(); err != nil {
			return nil, fmt.Errorf("sync recovered journal: %w", err)
		}
	}
	lines := bytes.Split(complete, []byte{'\n'})
	records := make([]Record, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("decode journal record %d: %w", len(records)+1, err)
		}
		expectedSequence := uint64(len(records) + 1)
		if err := validateRecord(record, expectedSequence); err != nil {
			return nil, fmt.Errorf("validate journal record %d: %w", expectedSequence, err)
		}
		records = append(records, cloneRecord(record))
	}
	return records, nil
}

func encodeRecord(sequence uint64, kind string, payload json.RawMessage) (Record, []byte, error) {
	unsigned := unsignedRecord{Schema: journalSchema, Sequence: sequence, Kind: kind, Payload: payload}
	encoded, err := json.Marshal(unsigned)
	if err != nil {
		return Record{}, nil, fmt.Errorf("encode journal record: %w", err)
	}
	digest := sha256.Sum256(encoded)
	record := Record{
		Schema: unsigned.Schema, Sequence: unsigned.Sequence, Kind: unsigned.Kind,
		Payload: append(json.RawMessage(nil), payload...), Checksum: hex.EncodeToString(digest[:]),
	}
	line, err := json.Marshal(record)
	if err != nil {
		return Record{}, nil, fmt.Errorf("encode checksummed journal record: %w", err)
	}
	return record, line, nil
}

func validateRecord(record Record, expectedSequence uint64) error {
	if record.Schema != journalSchema {
		return fmt.Errorf("schema %q is not supported", record.Schema)
	}
	if record.Sequence != expectedSequence {
		return fmt.Errorf("sequence is %d; want %d", record.Sequence, expectedSequence)
	}
	if record.Kind == "" || len(record.Payload) == 0 {
		return fmt.Errorf("kind and payload are required")
	}
	unsigned := unsignedRecord{
		Schema: record.Schema, Sequence: record.Sequence, Kind: record.Kind, Payload: record.Payload,
	}
	encoded, err := json.Marshal(unsigned)
	if err != nil {
		return fmt.Errorf("encode checksum input: %w", err)
	}
	digest := sha256.Sum256(encoded)
	if record.Checksum != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("checksum mismatch")
	}
	return nil
}

func writeFull(writer io.Writer, contents []byte) error {
	for len(contents) > 0 {
		written, err := writer.Write(contents)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		contents = contents[written:]
	}
	return nil
}

func cloneRecords(records []Record) []Record {
	cloned := make([]Record, len(records))
	for index, record := range records {
		cloned[index] = cloneRecord(record)
	}
	return cloned
}

func cloneRecord(record Record) Record {
	record.Payload = append(json.RawMessage(nil), record.Payload...)
	return record
}
