package gyms

import "context"

type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
}

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type EquipmentItem struct {
	EquipmentID string  `json:"equipment_id"`
	Label       string  `json:"label"`
	Quantity    *int    `json:"quantity"`
	Status      string  `json:"status"`
	VerifiedAt  *string `json:"verified_at"`
	SourceType  string  `json:"source_type"`
}

type Facility struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Category   string  `json:"category"`
	VerifiedAt *string `json:"verified_at,omitempty"`
	SourceType string  `json:"source_type,omitempty"`
}

type Summary struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Address             Address  `json:"address"`
	Location            Location `json:"location"`
	VerificationStatus  string   `json:"verification_status"`
	SourceType          string   `json:"source_type"`
	DistanceKm          *float64 `json:"distance_km,omitempty"`
	AverageRating       *float64 `json:"average_rating,omitempty"`
	ApprovedReviewCount int      `json:"approved_review_count"`
}

type Gym struct {
	Summary
	Timezone        string          `json:"timezone"`
	Contact         map[string]any  `json:"contact"`
	OpeningHours    map[string]any  `json:"opening_hours"`
	PriceInfo       map[string]any  `json:"price_info"`
	SourceReference *string         `json:"source_reference"`
	VerifiedAt      *string         `json:"verified_at"`
	LastUpdatedAt   string          `json:"last_updated_at"`
	Facilities      []string        `json:"facilities"`
	Equipment       []EquipmentItem `json:"equipment"`
}

type Search struct {
	Query              string
	Latitude           *float64
	Longitude          *float64
	RadiusKm           *float64
	Equipment          []string
	Facilities         []string
	VerificationStatus string
	Limit              int
	Offset             int
}

type Page struct {
	Items  []Summary
	Limit  int
	Offset int
	Total  int
}

type InventoryProvider interface {
	AvailableEquipment(ctx context.Context, gymID string) ([]string, string, error)
}
