package profiles

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"
)

const maxPresetEquipment = 32

type EquipmentPreset struct {
	Configured bool      `json:"configured"`
	Equipment  []string  `json:"equipment"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// GetEquipmentPreset is owner-scoped. A missing row means the user has never
// set a preset; a row with zero items is an explicitly empty preset.
func (s *Service) GetEquipmentPreset(ctx context.Context, userID string) (EquipmentPreset, error) {
	var updated string
	err := s.db.QueryRowContext(ctx, "SELECT updated_at FROM user_equipment_presets WHERE user_id=?", userID).Scan(&updated)
	if errors.Is(err, sql.ErrNoRows) {
		return EquipmentPreset{Equipment: []string{}}, nil
	}
	if err != nil {
		return EquipmentPreset{}, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT equipment_id FROM user_equipment_preset_items WHERE user_id=? ORDER BY equipment_id", userID)
	if err != nil {
		return EquipmentPreset{}, err
	}
	defer rows.Close()
	items := make([]string, 0, maxPresetEquipment)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return EquipmentPreset{}, err
		}
		items = append(items, id)
	}
	if err := rows.Err(); err != nil {
		return EquipmentPreset{}, err
	}
	stamp, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return EquipmentPreset{}, err
	}
	return EquipmentPreset{Configured: true, Equipment: items, UpdatedAt: stamp.UTC()}, nil
}

// PutEquipmentPreset atomically replaces the reusable preset. IDs are
// canonicalized by trimming and deduplicating, then stored in stable order.
func (s *Service) PutEquipmentPreset(ctx context.Context, userID string, equipment []string) error {
	if len(equipment) > maxPresetEquipment {
		return ErrInvalid
	}
	set := make(map[string]struct{}, len(equipment))
	for _, id := range equipment {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 80 {
			return ErrInvalid
		}
		set[id] = struct{}{}
	}
	if len(set) > maxPresetEquipment {
		return ErrInvalid
	}
	items := make([]string, 0, len(set))
	for id := range set {
		items = append(items, id)
	}
	sort.Strings(items)
	for _, id := range items {
		var exists int
		if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM equipment WHERE id=? AND active=1", id).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalid
			}
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_equipment_presets(user_id,updated_at) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET updated_at=excluded.updated_at`, userID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM user_equipment_preset_items WHERE user_id=?", userID); err != nil {
		return err
	}
	for _, id := range items {
		if _, err = tx.ExecContext(ctx, "INSERT INTO user_equipment_preset_items(user_id,equipment_id) VALUES(?,?)", userID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
