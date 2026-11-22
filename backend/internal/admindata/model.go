package admindata

import (
	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/gyms"
)

type Kind string

const (
	Gym       Kind = "gym"
	Exercise  Kind = "exercise"
	Equipment Kind = "equipment"
	Facility  Kind = "facility"
	MET       Kind = "met"
)

func ParseKind(value string) (Kind, bool) {
	kind := Kind(value)
	switch kind {
	case Gym, Exercise, Equipment, Facility, MET:
		return kind, true
	default:
		return "", false
	}
}

type ImportResult struct {
	Kind   Kind `json:"kind"`
	Count  int  `json:"count"`
	DryRun bool `json:"dry_run"`
}

type equipmentRecord struct {
	catalog.Equipment
	Active *bool `json:"active"`
}

type exerciseRecord struct {
	catalog.Exercise
	Active *bool `json:"active"`
}

type gymRecord struct {
	gyms.Gym
	Active *bool `json:"active"`
}

type gymExportRecord struct {
	gyms.Gym
	Active bool `json:"active"`
}

type facilityRecord struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
}
