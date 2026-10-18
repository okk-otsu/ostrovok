package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"travel-agent/internal/database"
)

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHotelSearchFiltersByMaximumPrice(t *testing.T) {
	result, err := NewHotelSearch(testDatabase(t)).Execute(context.Background(), json.RawMessage(`{
		"city":"Казань",
		"check_in":"2026-11-07",
		"check_out":"2026-11-09",
		"guests":2,
		"max_price":10000
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var decoded struct {
		Hotels []struct {
			TotalPrice int `json:"total_price"`
		} `json:"hotels"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(decoded.Hotels) != 1 || decoded.Hotels[0].TotalPrice != 8500 {
		t.Fatalf("hotels = %+v, want only the 8500-ruble option", decoded.Hotels)
	}
}

func TestTransportSearchRejectsInvalidPassengerCount(t *testing.T) {
	_, err := NewTransportSearch(testDatabase(t)).Execute(context.Background(), json.RawMessage(`{
		"from":"Москва",
		"to":"Казань",
		"departure_date":"2026-11-07",
		"passengers":0
	}`))
	if err == nil {
		t.Fatal("Execute() error = nil, want validation error")
	}
}

func TestHotelSearchRequiresMaximumPrice(t *testing.T) {
	_, err := NewHotelSearch(testDatabase(t)).Execute(context.Background(), json.RawMessage(`{
		"city":"Казань",
		"check_in":"2026-11-07",
		"check_out":"2026-11-09",
		"guests":2
	}`))
	if err == nil {
		t.Fatal("Execute() error = nil, want missing max_price error")
	}
}

func TestWeatherReturnsMockForecast(t *testing.T) {
	result, err := NewWeather().Execute(context.Background(), json.RawMessage(`{"city":"Казань","date":"2026-11-07"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !json.Valid(payload) {
		t.Fatalf("tool returned invalid JSON: %s", payload)
	}
}
