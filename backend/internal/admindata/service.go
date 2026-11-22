package admindata

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	maxImportBytes   = 16 << 20
	maxImportRecords = 10000
)

var ErrInvalidDataset = errors.New("invalid reference data dataset")

type cacheInvalidator interface{ InvalidateCache() }

type Service struct {
	db      *sql.DB
	catalog cacheInvalidator
	gyms    cacheInvalidator
}

func New(db *sql.DB, catalogCache, gymCache cacheInvalidator) *Service {
	return &Service{db: db, catalog: catalogCache, gyms: gymCache}
}

func (s *Service) Import(ctx context.Context, kind Kind, reader io.Reader, dryRun bool) (ImportResult, error) {
	if s.db == nil {
		return ImportResult{}, fmt.Errorf("reference-data database is unavailable")
	}
	if _, ok := ParseKind(string(kind)); !ok {
		return ImportResult{}, fmt.Errorf("unsupported reference-data type %q", kind)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxImportBytes+1))
	if err != nil {
		return ImportResult{}, fmt.Errorf("read %s dataset: %w", kind, err)
	}
	if len(data) > maxImportBytes {
		return ImportResult{}, fmt.Errorf("%s dataset exceeds the %d-byte limit", kind, maxImportBytes)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, err
	}
	defer tx.Rollback()
	count, err := s.importDataset(ctx, tx, kind, data)
	if err != nil {
		return ImportResult{}, err
	}
	if dryRun {
		return ImportResult{Kind: kind, Count: count, DryRun: true}, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reference_data_revision SET revision=revision+1 WHERE id=1`); err != nil {
		return ImportResult{}, fmt.Errorf("mark reference data changed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("commit %s import: %w", kind, err)
	}
	if s.catalog != nil {
		s.catalog.InvalidateCache()
	}
	if s.gyms != nil {
		s.gyms.InvalidateCache()
	}
	return ImportResult{Kind: kind, Count: count}, nil
}

func decodeDataset[T any](kind Kind, data []byte) ([]T, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var records []T
	if err := decoder.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode %s dataset: %w", kind, err)
	}
	if records == nil || len(records) == 0 {
		return nil, fmt.Errorf("%w: %s dataset must be a non-empty JSON array", ErrInvalidDataset, kind)
	}
	if len(records) > maxImportRecords {
		return nil, fmt.Errorf("%s dataset exceeds the %d-record limit", kind, maxImportRecords)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("decode %s dataset: multiple JSON values", kind)
		}
		return nil, fmt.Errorf("decode %s dataset trailing data: %w", kind, err)
	}
	return records, nil
}

func (s *Service) importDataset(ctx context.Context, tx *sql.Tx, kind Kind, data []byte) (int, error) {
	switch kind {
	case Gym:
		return importGyms(ctx, tx, data)
	case Exercise:
		return importExercises(ctx, tx, data)
	case Equipment:
		return importEquipment(ctx, tx, data)
	case Facility:
		return importFacilities(ctx, tx, data)
	case MET:
		return importMET(ctx, tx, data)
	default:
		return 0, fmt.Errorf("unsupported reference-data type %q", kind)
	}
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
