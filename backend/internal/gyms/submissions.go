package gyms

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

var ErrSubmissionInvalid = errors.New("invalid gym submission")
var ErrSubmissionNotFound = errors.New("gym submission not found")

type Submission struct {
	ID           string          `json:"id"`
	GymID        *string         `json:"gym_id,omitempty"`
	Type         string          `json:"submission_type"`
	Changes      json.RawMessage `json:"changes"`
	Evidence     []string        `json:"evidence"`
	Status       string          `json:"status"`
	ReviewReason string          `json:"review_reason,omitempty"`
	CreatedAt    string          `json:"created_at"`
}

func (s *Service) Submit(ctx context.Context, userID, kind, gymID string, changes map[string]any, evidence []string) (Submission, error) {
	if kind != "new_gym" && kind != "correction" && kind != "equipment_correction" || len(changes) == 0 || len(evidence) > 10 {
		return Submission{}, ErrSubmissionInvalid
	}
	if kind == "new_gym" && gymID != "" || kind != "new_gym" && gymID == "" {
		return Submission{}, ErrSubmissionInvalid
	}
	if len(gymID) > 80 {
		return Submission{}, ErrSubmissionInvalid
	}
	for _, e := range evidence {
		if len(e) > 500 || strings.ContainsAny(e, "\r\n") {
			return Submission{}, ErrSubmissionInvalid
		}
	}
	if gymID != "" {
		var n int
		if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM gyms WHERE id=? AND active=1", gymID).Scan(&n); err != nil {
			return Submission{}, ErrSubmissionInvalid
		}
	}
	changesJSON, _ := json.Marshal(changes)
	evidenceJSON, _ := json.Marshal(evidence)
	id, err := newSubmissionID()
	if err != nil {
		return Submission{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, "INSERT INTO gym_submissions(id,gym_id,submitted_by,submission_type,changes_json,evidence_json,status,created_at) VALUES(?,?,?,?,?,?,'pending',?)", id, nullIfEmpty(gymID), userID, kind, string(changesJSON), string(evidenceJSON), now)
	if err != nil {
		return Submission{}, err
	}
	var gymPtr *string
	if gymID != "" {
		gymPtr = &gymID
	}
	return Submission{ID: id, GymID: gymPtr, Type: kind, Changes: changesJSON, Evidence: evidence, Status: "pending", CreatedAt: now}, nil
}
func (s *Service) GetSubmission(ctx context.Context, id, userID string, admin bool) (Submission, error) {
	query := `SELECT id,gym_id,submission_type,changes_json,evidence_json,status,COALESCE(review_reason,''),created_at FROM gym_submissions WHERE id=?`
	args := []any{id}
	if !admin {
		query += ` AND submitted_by=?`
		args = append(args, userID)
	}
	var v Submission
	var gymID sql.NullString
	var changes, evidence string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&v.ID, &gymID, &v.Type, &changes, &evidence, &v.Status, &v.ReviewReason, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrSubmissionNotFound
	}
	if err != nil {
		return v, err
	}
	if gymID.Valid {
		v.GymID = &gymID.String
	}
	v.Changes = json.RawMessage(changes)
	err = json.Unmarshal([]byte(evidence), &v.Evidence)
	return v, err
}
func (s *Service) ListSubmissions(ctx context.Context, status string) ([]Submission, error) {
	if status != "" && status != "pending" && status != "approved" && status != "rejected" {
		return nil, ErrSubmissionInvalid
	}
	query := `SELECT id,gym_id,submission_type,changes_json,evidence_json,status,COALESCE(review_reason,''),created_at FROM gym_submissions`
	args := []any{}
	if status != "" {
		query += " WHERE status=?"
		args = append(args, status)
	}
	query += " ORDER BY created_at LIMIT 200"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Submission, 0)
	for rows.Next() {
		var v Submission
		var gymID sql.NullString
		var changes, evidence string
		if err = rows.Scan(&v.ID, &gymID, &v.Type, &changes, &evidence, &v.Status, &v.ReviewReason, &v.CreatedAt); err != nil {
			return nil, err
		}
		if gymID.Valid {
			v.GymID = &gymID.String
		}
		v.Changes = json.RawMessage(changes)
		if err = json.Unmarshal([]byte(evidence), &v.Evidence); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (s *Service) Review(ctx context.Context, submissionID, adminID, decision, reason string) error {
	if decision != "approve" && decision != "reject" || len(reason) > 1000 {
		return ErrSubmissionInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var gymID sql.NullString
	var kind, changesJSON string
	if err = tx.QueryRowContext(ctx, "SELECT gym_id,submission_type,changes_json FROM gym_submissions WHERE id=? AND status='pending'", submissionID).Scan(&gymID, &kind, &changesJSON); errors.Is(err, sql.ErrNoRows) {
		return ErrSubmissionNotFound
	} else if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if decision == "approve" {
		if kind == "new_gym" {
			var createdID string
			createdID, err = applyNewGym(ctx, tx, changesJSON, now)
			if err != nil {
				return err
			}
			gymID = sql.NullString{String: createdID, Valid: true}
		} else if kind == "equipment_correction" {
			if !gymID.Valid {
				return ErrSubmissionInvalid
			}
			if err = applyInventory(ctx, tx, gymID.String, changesJSON, now); err != nil {
				return err
			}
		} else {
			if !gymID.Valid {
				return ErrSubmissionInvalid
			}
			if err = applyGymCorrection(ctx, tx, gymID.String, changesJSON, now); err != nil {
				return err
			}
		}
	}
	reviewStatus := "rejected"
	if decision == "approve" {
		reviewStatus = "approved"
	}
	if _, err = tx.ExecContext(ctx, "UPDATE gym_submissions SET gym_id=?,status=?,reviewed_by=?,review_reason=?,reviewed_at=? WHERE id=? AND status='pending'", nullIfEmpty(gymID.String), reviewStatus, adminID, reason, now, submissionID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,actor_user_id,action,entity_type,entity_id,timestamp,details_json) VALUES(?,?,?,?,?,?,?)", mustID(), adminID, "gym_submission_"+reviewStatus, "gym_submission", submissionID, now, `{"reason":`+mustJSON(reason)+`}`); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}

type newGymChanges struct {
	Name       string            `json:"name"`
	Street     string            `json:"street"`
	City       string            `json:"city"`
	Region     string            `json:"region"`
	Country    string            `json:"country"`
	Latitude   float64           `json:"latitude"`
	Longitude  float64           `json:"longitude"`
	Timezone   string            `json:"timezone"`
	Facilities []string          `json:"facilities"`
	Equipment  []inventoryChange `json:"equipment"`
}
type inventoryChange struct {
	EquipmentID string `json:"equipment_id"`
	Quantity    *int   `json:"quantity"`
	Status      string `json:"status"`
}

func applyNewGym(ctx context.Context, tx *sql.Tx, raw, now string) (string, error) {
	var v newGymChanges
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", ErrSubmissionInvalid
	}
	if len(v.Name) < 2 || len(v.Name) > 160 || len(v.City) == 0 || len(v.Country) == 0 || v.Latitude < -90 || v.Latitude > 90 || v.Longitude < -180 || v.Longitude > 180 || math.IsNaN(v.Latitude) || math.IsNaN(v.Longitude) || v.Timezone == "" {
		return "", ErrSubmissionInvalid
	}
	address, _ := json.Marshal(Address{Street: v.Street, City: v.City, Region: v.Region, Country: v.Country})
	id := mustID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO gyms(id,name,address_json,latitude,longitude,timezone,verification_status,source_type,last_updated_at,active) VALUES(?,?,?,?,?,?,'unverified','user_submission',?,1)`, id, v.Name, string(address), v.Latitude, v.Longitude, v.Timezone, now); err != nil {
		return "", err
	}
	if err := applyInventoryValues(ctx, tx, id, v.Equipment, now); err != nil {
		return "", err
	}
	for _, facility := range v.Facilities {
		if _, err := tx.ExecContext(ctx, "INSERT INTO gym_facilities(gym_id,facility_id,source_type) VALUES(?,?,'user_submission')", id, facility); err != nil {
			return "", err
		}
	}
	return id, nil
}
func applyInventory(ctx context.Context, tx *sql.Tx, gymID, raw, now string) error {
	var v struct {
		Equipment []inventoryChange `json:"equipment"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil || len(v.Equipment) == 0 {
		return ErrSubmissionInvalid
	}
	return applyInventoryValues(ctx, tx, gymID, v.Equipment, now)
}
func applyInventoryValues(ctx context.Context, tx *sql.Tx, gymID string, items []inventoryChange, now string) error {
	if len(items) > 100 {
		return ErrSubmissionInvalid
	}
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if item.Status != "present" && item.Status != "out_of_service" && item.Status != "unknown" || item.Quantity != nil && *item.Quantity < 0 {
			return ErrSubmissionInvalid
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO gym_equipment(gym_id,equipment_id,quantity,status,source_type,verified_at) VALUES(?,?,?,?, 'user_submission',NULL) ON CONFLICT(gym_id,equipment_id) DO UPDATE SET quantity=excluded.quantity,status=excluded.status,source_type='user_submission',verified_at=NULL`, gymID, item.EquipmentID, item.Quantity, item.Status); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, "UPDATE gyms SET verification_status=CASE WHEN verification_status='unverified' THEN 'unverified' ELSE 'stale' END,last_updated_at=? WHERE id=?", now, gymID)
	return err
}
func applyGymCorrection(ctx context.Context, tx *sql.Tx, gymID, raw, now string) error {
	var changes map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &changes); err != nil || len(changes) == 0 {
		return ErrSubmissionInvalid
	}
	for key, value := range changes {
		if key != "name" {
			return ErrSubmissionInvalid
		}
		var name string
		if json.Unmarshal(value, &name) != nil || len(name) < 2 || len(name) > 160 {
			return ErrSubmissionInvalid
		}
		if _, err := tx.ExecContext(ctx, "UPDATE gyms SET name=?,verification_status='stale',last_updated_at=? WHERE id=?", name, now, gymID); err != nil {
			return err
		}
	}
	return nil
}
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func newSubmissionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
func mustID() string           { id, _ := newSubmissionID(); return id }
func mustJSON(v string) string { raw, _ := json.Marshal(v); return string(raw) }

func (s *Service) AdminUpdate(ctx context.Context, gymID, adminID string, changes map[string]any) error {
	if len(changes) == 0 {
		return ErrSubmissionInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name, addressRaw string
	var lat, lon float64
	if err = tx.QueryRowContext(ctx, "SELECT name,address_json,latitude,longitude FROM gyms WHERE id=? AND active=1", gymID).Scan(&name, &addressRaw, &lat, &lon); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	address := Address{}
	if err = json.Unmarshal([]byte(addressRaw), &address); err != nil {
		return err
	}
	for key, value := range changes {
		switch key {
		case "name":
			v, ok := value.(string)
			if !ok || len(v) < 2 || len(v) > 160 {
				return ErrSubmissionInvalid
			}
			name = v
		case "address":
			raw, e := json.Marshal(value)
			if e != nil || json.Unmarshal(raw, &address) != nil || address.City == "" || address.Country == "" {
				return ErrSubmissionInvalid
			}
		case "latitude":
			v, ok := value.(float64)
			if !ok || v < -90 || v > 90 || math.IsNaN(v) {
				return ErrSubmissionInvalid
			}
			lat = v
		case "longitude":
			v, ok := value.(float64)
			if !ok || v < -180 || v > 180 || math.IsNaN(v) {
				return ErrSubmissionInvalid
			}
			lon = v
		default:
			return ErrSubmissionInvalid
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	addressJSON, _ := json.Marshal(address)
	if _, err = tx.ExecContext(ctx, "UPDATE gyms SET name=?,address_json=?,latitude=?,longitude=?,verification_status='verified',source_type='admin',verified_at=?,last_updated_at=? WHERE id=?", name, string(addressJSON), lat, lon, now, now, gymID); err != nil {
		return err
	}
	detail, _ := json.Marshal(changes)
	if _, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,actor_user_id,action,entity_type,entity_id,timestamp,details_json) VALUES(?,?, 'gym_admin_update','gym',?,?,?)", mustID(), adminID, gymID, now, string(detail)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}
