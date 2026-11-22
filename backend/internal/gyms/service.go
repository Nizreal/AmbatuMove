package gyms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("gym not found")
var ErrInvalidSearch = errors.New("invalid gym search")

type Service struct {
	db    *sql.DB
	cache *gymReadCache
}

func NewService(db *sql.DB) *Service            { return &Service{db: db, cache: newGymReadCache()} }
func (s *Service) CacheStats() CacheStats       { return s.cache.stats() }
func (s *Service) SetCacheEnabled(enabled bool) { s.cache.setEnabled(enabled) }
func (s *Service) InvalidateCache()             { s.cache.invalidate() }
func (s *Service) refreshCacheRevision(ctx context.Context) error {
	var revision int64
	if err := s.db.QueryRowContext(ctx, "SELECT revision FROM reference_data_revision WHERE id=1").Scan(&revision); err != nil {
		return err
	}
	s.cache.setRevision(revision)
	return nil
}

func (s *Service) Search(ctx context.Context, request Search) (Page, error) {
	if err := s.refreshCacheRevision(ctx); err != nil {
		return Page{}, err
	}
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Limit < 1 || request.Limit > 100 || request.Offset < 0 || request.Offset > 1000000 || len(request.Query) > 120 {
		return Page{}, ErrInvalidSearch
	}
	if len(request.Equipment) > 20 || len(request.Facilities) > 20 {
		return Page{}, ErrInvalidSearch
	}
	for _, id := range append(append([]string{}, request.Equipment...), request.Facilities...) {
		if len(id) > 80 {
			return Page{}, ErrInvalidSearch
		}
	}
	if (request.Latitude == nil) != (request.Longitude == nil) || request.RadiusKm != nil && request.Latitude == nil {
		return Page{}, ErrInvalidSearch
	}
	if request.Latitude != nil && (*request.Latitude < -90 || *request.Latitude > 90 || *request.Longitude < -180 || *request.Longitude > 180 || math.IsNaN(*request.Latitude) || math.IsNaN(*request.Longitude) || math.IsInf(*request.Latitude, 0) || math.IsInf(*request.Longitude, 0)) {
		return Page{}, ErrInvalidSearch
	}
	if request.RadiusKm != nil && (*request.RadiusKm <= 0 || *request.RadiusKm > 100 || math.IsNaN(*request.RadiusKm) || math.IsInf(*request.RadiusKm, 0)) {
		return Page{}, ErrInvalidSearch
	}
	if request.VerificationStatus != "" && request.VerificationStatus != "verified" && request.VerificationStatus != "unverified" && request.VerificationStatus != "stale" {
		return Page{}, ErrInvalidSearch
	}
	if err := s.validateFilters(ctx, request); err != nil {
		return Page{}, err
	}
	request.Equipment = append([]string(nil), request.Equipment...)
	request.Facilities = append([]string(nil), request.Facilities...)
	sort.Strings(request.Equipment)
	sort.Strings(request.Facilities)
	keyBytes, _ := json.Marshal(request)
	cacheKey := "search:" + string(keyBytes)
	var cached Page
	if s.cache.get(cacheKey, &cached) {
		return cached, nil
	}
	where, args := gymWhere(request)
	var summaries []Summary
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gyms g WHERE "+where, args...).Scan(&total); err != nil {
		return Page{}, err
	}
	query := `SELECT g.id,g.name,g.address_json,g.latitude,g.longitude,g.verification_status,g.source_type FROM gyms g WHERE ` + where
	queryArgs := append([]any{}, args...)
	if request.Latitude != nil {
		latDelta := *request.RadiusKmOrDefault() / 111.0
		cosine := math.Abs(math.Cos(*request.Latitude * math.Pi / 180))
		lonDelta := 180.0
		if cosine > 0.01 {
			lonDelta = math.Min(180, latDelta/cosine)
		}
		query += " AND g.latitude BETWEEN ? AND ? AND g.longitude BETWEEN ? AND ?"
		queryArgs = append(queryArgs, maxFloatValue(-90, *request.Latitude-latDelta), minFloatValue(90, *request.Latitude+latDelta), maxFloatValue(-180, *request.Longitude-lonDelta), minFloatValue(180, *request.Longitude+lonDelta))
	}
	query += " ORDER BY g.name COLLATE NOCASE,g.id"
	if request.Latitude == nil {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, request.Limit, request.Offset)
	} else {
		query += " LIMIT 10000"
	}
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return Page{}, err
	}
	summaries = make([]Summary, 0)
	for rows.Next() {
		var item Summary
		var addressJSON string
		if err := rows.Scan(&item.ID, &item.Name, &addressJSON, &item.Location.Latitude, &item.Location.Longitude, &item.VerificationStatus, &item.SourceType); err != nil {
			_ = rows.Close()
			return Page{}, err
		}
		if err := json.Unmarshal([]byte(addressJSON), &item.Address); err != nil {
			_ = rows.Close()
			return Page{}, fmt.Errorf("decode gym address: %w", err)
		}
		if request.Latitude != nil {
			distance := haversineKm(*request.Latitude, *request.Longitude, item.Location.Latitude, item.Location.Longitude)
			if distance > *request.RadiusKmOrDefault() {
				continue
			}
			item.DistanceKm = &distance
		}
		summaries = append(summaries, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Page{}, err
	}
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	if request.Latitude != nil {
		sort.Slice(summaries, func(i, j int) bool {
			if *summaries[i].DistanceKm == *summaries[j].DistanceKm {
				return summaries[i].ID < summaries[j].ID
			}
			return *summaries[i].DistanceKm < *summaries[j].DistanceKm
		})
		total = len(summaries)
		start := minInt(request.Offset, len(summaries))
		end := minInt(start+request.Limit, len(summaries))
		summaries = summaries[start:end]
	}
	page := Page{Items: summaries, Limit: request.Limit, Offset: request.Offset, Total: total}
	s.cache.put(cacheKey, page)
	return page, nil
}

func (r Search) RadiusKmOrDefault() *float64 {
	if r.RadiusKm != nil {
		return r.RadiusKm
	}
	defaultRadius := 10.0
	return &defaultRadius
}

func (s *Service) Get(ctx context.Context, id string) (Gym, error) {
	if err := s.refreshCacheRevision(ctx); err != nil {
		return Gym{}, err
	}
	var cached Gym
	if s.cache.get("gym:"+id, &cached) {
		return cached, nil
	}
	var result Gym
	var address, contact string
	var opening, price, sourceRef, verified sql.NullString
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT id,name,address_json,latitude,longitude,timezone,contact_json,opening_hours_json,price_json,verification_status,source_type,source_reference,verified_at,last_updated_at,active FROM gyms WHERE id=? AND active=1`, id).Scan(
		&result.ID, &result.Name, &address, &result.Location.Latitude, &result.Location.Longitude, &result.Timezone, &contact, &opening, &price, &result.VerificationStatus, &result.SourceType, &sourceRef, &verified, &result.LastUpdatedAt, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return Gym{}, ErrNotFound
	}
	if err != nil {
		return Gym{}, err
	}
	if err := json.Unmarshal([]byte(address), &result.Address); err != nil {
		return Gym{}, err
	}
	if err := json.Unmarshal([]byte(contact), &result.Contact); err != nil {
		return Gym{}, err
	}
	if opening.Valid {
		_ = json.Unmarshal([]byte(opening.String), &result.OpeningHours)
	}
	if price.Valid {
		_ = json.Unmarshal([]byte(price.String), &result.PriceInfo)
	}
	if sourceRef.Valid {
		result.SourceReference = &sourceRef.String
	}
	if verified.Valid {
		result.VerifiedAt = &verified.String
	}
	result.Summary = Summary{ID: result.ID, Name: result.Name, Address: result.Address, Location: result.Location, VerificationStatus: result.VerificationStatus, SourceType: result.SourceType}
	result.Facilities, err = s.facilityIDs(ctx, id)
	if err != nil {
		return Gym{}, err
	}
	result.Equipment, err = s.Equipment(ctx, id)
	if err != nil {
		return Gym{}, err
	}
	result.Summary.AverageRating, result.Summary.ApprovedReviewCount, err = s.ApprovedRating(ctx, id)
	if err != nil {
		return Gym{}, err
	}
	s.cache.put("gym:"+id, result)
	return result, nil
}

func (s *Service) Equipment(ctx context.Context, gymID string) ([]EquipmentItem, error) {
	if err := s.refreshCacheRevision(ctx); err != nil {
		return nil, err
	}
	var cached []EquipmentItem
	if s.cache.get("equipment:"+gymID, &cached) {
		return cached, nil
	}
	if _, err := s.GetID(ctx, gymID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ge.equipment_id,e.label,ge.quantity,ge.status,ge.verified_at,ge.source_type FROM gym_equipment ge JOIN equipment e ON e.id=ge.equipment_id WHERE ge.gym_id=? ORDER BY e.label COLLATE NOCASE,e.id`, gymID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EquipmentItem, 0)
	for rows.Next() {
		var item EquipmentItem
		var quantity sql.NullInt64
		var verified sql.NullString
		if err := rows.Scan(&item.EquipmentID, &item.Label, &quantity, &item.Status, &verified, &item.SourceType); err != nil {
			return nil, err
		}
		if quantity.Valid {
			value := int(quantity.Int64)
			item.Quantity = &value
		}
		if verified.Valid {
			item.VerifiedAt = &verified.String
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	s.cache.put("equipment:"+gymID, items)
	return items, nil
}

func (s *Service) Facilities(ctx context.Context, gymID string) ([]Facility, error) {
	if err := s.refreshCacheRevision(ctx); err != nil {
		return nil, err
	}
	var cached []Facility
	if s.cache.get("facilities:"+gymID, &cached) {
		return cached, nil
	}
	if _, err := s.GetID(ctx, gymID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT f.id,f.label,f.category,gf.verified_at,gf.source_type FROM gym_facilities gf JOIN facilities f ON f.id=gf.facility_id WHERE gf.gym_id=? ORDER BY f.label COLLATE NOCASE,f.id`, gymID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Facility, 0)
	for rows.Next() {
		var item Facility
		var verified sql.NullString
		if err := rows.Scan(&item.ID, &item.Label, &item.Category, &verified, &item.SourceType); err != nil {
			return nil, err
		}
		if verified.Valid {
			item.VerifiedAt = &verified.String
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	s.cache.put("facilities:"+gymID, items)
	return items, nil
}

func (s *Service) ListFacilities(ctx context.Context) ([]Facility, error) {
	if err := s.refreshCacheRevision(ctx); err != nil {
		return nil, err
	}
	var cached []Facility
	if s.cache.get("facilities:vocabulary", &cached) {
		return cached, nil
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,label,category FROM facilities ORDER BY label COLLATE NOCASE,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Facility, 0)
	for rows.Next() {
		var item Facility
		if err := rows.Scan(&item.ID, &item.Label, &item.Category); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	s.cache.put("facilities:vocabulary", items)
	return items, nil
}

func (s *Service) AvailableEquipment(ctx context.Context, gymID string) ([]string, string, error) {
	if _, err := s.GetID(ctx, gymID); err != nil {
		return nil, "", err
	}
	var verification string
	if err := s.db.QueryRowContext(ctx, "SELECT verification_status FROM gyms WHERE id=? AND active=1", gymID).Scan(&verification); err != nil {
		return nil, "", err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT equipment_id FROM gym_equipment WHERE gym_id=? AND status='present' ORDER BY equipment_id", gymID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", err
		}
		ids = append(ids, id)
	}
	return ids, verification, rows.Err()
}

func (s *Service) GetID(ctx context.Context, gymID string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "SELECT id FROM gyms WHERE id=? AND active=1", gymID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (s *Service) validateFilters(ctx context.Context, request Search) error {
	for _, id := range request.Equipment {
		var found int
		if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM equipment WHERE id=? AND active=1", id).Scan(&found); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidSearch
			}
			return err
		}
	}
	for _, id := range request.Facilities {
		var found int
		if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM facilities WHERE id=?", id).Scan(&found); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidSearch
			}
			return err
		}
	}
	return nil
}

func gymWhere(request Search) (string, []any) {
	clauses := []string{"g.active=1"}
	args := make([]any, 0, 4+len(request.Equipment)+len(request.Facilities))
	if request.Query != "" {
		clauses = append(clauses, "lower(g.name) LIKE ?")
		args = append(args, "%"+strings.ToLower(request.Query)+"%")
	}
	if request.VerificationStatus != "" {
		clauses = append(clauses, "g.verification_status=?")
		args = append(args, request.VerificationStatus)
	}
	for _, id := range request.Equipment {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM gym_equipment ge WHERE ge.gym_id=g.id AND ge.equipment_id=? AND ge.status='present')")
		args = append(args, id)
	}
	for _, id := range request.Facilities {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM gym_facilities gf WHERE gf.gym_id=g.id AND gf.facility_id=?)")
		args = append(args, id)
	}
	return strings.Join(clauses, " AND "), args
}

func (s *Service) facilityIDs(ctx context.Context, gymID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT facility_id FROM gym_facilities WHERE gym_id=? ORDER BY facility_id", gymID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

var defaultFacilities = []Facility{{ID: "shower", Label: "Shower", Category: "amenity"}, {ID: "locker", Label: "Locker", Category: "amenity"}, {ID: "parking", Label: "Parking", Category: "amenity"}, {ID: "drinking_water", Label: "Drinking water", Category: "amenity"}, {ID: "accessible_entrance", Label: "Accessible entrance", Category: "accessibility"}}

// SeedFacilities initializes missing facility vocabulary without replacing
// values maintained by an administrator.
func SeedFacilities(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range defaultFacilities {
		if _, err := tx.ExecContext(ctx, "INSERT INTO facilities(id,label,category) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", item.ID, item.Label, item.Category); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeactivateSynthetic preserves fixture rows and their references for local
// reproducibility while ensuring they cannot appear in public results.
func (s *Service) DeactivateSynthetic(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE gyms SET active=0 WHERE source_type='synthetic' AND active<>0"); err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}

// SeedSyntheticFixtures creates opt-in local fixtures. Existing rows and their
// inventories are preserved so repeated development startup cannot overwrite
// edits made in an administrator workflow.
func (s *Service) SeedSyntheticFixtures(ctx context.Context) error {
	if err := SeedFacilities(ctx, s.db); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fixtures := []struct {
		id, name, city      string
		latitude, longitude float64
		equipment           []struct {
			id       string
			quantity any
			status   string
		}
		facilities []string
	}{
		{"gym_demo_jakarta_001", "Synthetic Demo Gym Jakarta", "Jakarta", -6.2000, 106.8167, []struct {
			id       string
			quantity any
			status   string
		}{{"cable_machine", 2, "present"}, {"dumbbell", nil, "present"}, {"flat_bench", 4, "present"}, {"barbell", 1, "present"}}, []string{"locker", "shower", "parking"}},
		{"gym_demo_jakarta_002", "Synthetic Demo Movement Studio", "Jakarta", -6.2100, 106.8200, []struct {
			id       string
			quantity any
			status   string
		}{{"bodyweight", nil, "present"}, {"stationary_bike", 4, "present"}, {"treadmill", 2, "out_of_service"}, {"yoga_mat", 8, "present"}}, []string{"drinking_water", "accessible_entrance"}},
		{"gym_demo_bandung_001", "Synthetic Demo Gym Bandung", "Bandung", -6.9175, 107.6191, []struct {
			id       string
			quantity any
			status   string
		}{{"kettlebell", 3, "present"}, {"resistance_band", nil, "present"}, {"pull_up_bar", 1, "present"}}, []string{"locker", "drinking_water"}},
	}
	for _, fixture := range fixtures {
		address, _ := json.Marshal(Address{Street: "Synthetic demonstration address", City: fixture.city, Region: "Synthetic region", Country: "Indonesia"})
		contact, _ := json.Marshal(map[string]any{"phone": nil, "website_url": nil})
		if _, err := tx.ExecContext(ctx, `INSERT INTO gyms(id,name,address_json,latitude,longitude,timezone,contact_json,verification_status,source_type,source_reference,verified_at,last_updated_at,active) VALUES(?,?,?,?,?,'Asia/Jakarta',?,'unverified','synthetic','Synthetic fixture; not a real surveyed business',NULL,'2026-11-21T00:00:00Z',1) ON CONFLICT(id) DO NOTHING`, fixture.id, fixture.name, string(address), fixture.latitude, fixture.longitude, string(contact)); err != nil {
			return fmt.Errorf("seed gym %s: %w", fixture.id, err)
		}
		for _, item := range fixture.equipment {
			if _, err := tx.ExecContext(ctx, `INSERT INTO gym_equipment(gym_id,equipment_id,quantity,status,verified_at,source_type) VALUES(?,?,?, ?,NULL,'synthetic') ON CONFLICT(gym_id,equipment_id) DO NOTHING`, fixture.id, item.id, item.quantity, item.status); err != nil {
				return err
			}
		}
		for _, facilityID := range fixture.facilities {
			if _, err := tx.ExecContext(ctx, `INSERT INTO gym_facilities(gym_id,facility_id,verified_at,source_type) VALUES(?,?,NULL,'synthetic') ON CONFLICT(gym_id,facility_id) DO NOTHING`, fixture.id, facilityID); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371.0088
	toRadians := func(value float64) float64 { return value * math.Pi / 180 }
	dLat, dLon := toRadians(lat2-lat1), toRadians(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(toRadians(lat1))*math.Cos(toRadians(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func maxFloatValue(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func minFloatValue(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
