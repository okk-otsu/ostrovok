package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type hotelSearch struct{ db *sql.DB }

type hotelSearchInput struct {
	City     string   `json:"city"`
	CheckIn  string   `json:"check_in"`
	CheckOut string   `json:"check_out"`
	Guests   int      `json:"guests"`
	MaxPrice *float64 `json:"max_price"`
}

func NewHotelSearch(db *sql.DB) Tool { return hotelSearch{db: db} }

func (hotelSearch) Name() string { return "search_hotels" }

func (hotelSearch) Description() string {
	return "Найти несколько вариантов отелей в городе с учетом дат, гостей и максимальной цены."
}

func (hotelSearch) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"city":{"type":"string","description":"Город проживания"},
			"check_in":{"type":"string","description":"Дата заезда, YYYY-MM-DD"},
			"check_out":{"type":"string","description":"Дата выезда, YYYY-MM-DD"},
			"guests":{"type":"integer","minimum":1,"description":"Количество гостей"},
			"max_price":{"type":"number","minimum":0,"description":"Максимальная цена за весь период в рублях"}
		},
		"required":["city","check_in","check_out","guests","max_price"],
		"additionalProperties":false
	}`)
}

func (h hotelSearch) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var input hotelSearchInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return nil, fmt.Errorf("parse search_hotels arguments: %w", err)
	}
	if strings.TrimSpace(input.City) == "" || strings.TrimSpace(input.CheckIn) == "" || strings.TrimSpace(input.CheckOut) == "" {
		return nil, fmt.Errorf("city, check_in and check_out are required")
	}
	if input.Guests < 1 || input.MaxPrice == nil || *input.MaxPrice < 0 {
		return nil, fmt.Errorf("guests must be positive and max_price is required and must not be negative")
	}
	checkIn, err := time.Parse("2006-01-02", input.CheckIn)
	if err != nil {
		return nil, fmt.Errorf("check_in must use YYYY-MM-DD")
	}
	checkOut, err := time.Parse("2006-01-02", input.CheckOut)
	if err != nil || !checkOut.After(checkIn) {
		return nil, fmt.Errorf("check_out must use YYYY-MM-DD and be after check_in")
	}
	nights := int(checkOut.Sub(checkIn).Hours() / 24)

	rows, err := h.db.QueryContext(ctx, `SELECT name, price_per_night, rating, price_per_night * ? AS total_price
		FROM hotels
		WHERE city = ? AND max_guests >= ? AND price_per_night * ? <= ?
		ORDER BY total_price, rating DESC`, nights, strings.TrimSpace(input.City), input.Guests, nights, *input.MaxPrice)
	if err != nil {
		return nil, fmt.Errorf("query hotels: %w", err)
	}
	defer rows.Close()

	hotels := make([]map[string]any, 0)
	for rows.Next() {
		var name string
		var pricePerNight, totalPrice int
		var rating float64
		if err := rows.Scan(&name, &pricePerNight, &rating, &totalPrice); err != nil {
			return nil, fmt.Errorf("read hotel: %w", err)
		}
		hotels = append(hotels, map[string]any{
			"name": name, "price_per_night": pricePerNight, "total_price": totalPrice, "rating": rating,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read hotels: %w", err)
	}
	return map[string]any{
		"city":      input.City,
		"check_in":  input.CheckIn,
		"check_out": input.CheckOut,
		"nights":    nights,
		"hotels":    hotels,
		"source":    "local_database",
	}, nil
}

type transportSearch struct{ db *sql.DB }

type transportSearchInput struct {
	From          string `json:"from"`
	To            string `json:"to"`
	DepartureDate string `json:"departure_date"`
	Passengers    int    `json:"passengers"`
}

func NewTransportSearch(db *sql.DB) Tool { return transportSearch{db: db} }

func (transportSearch) Name() string { return "search_transport" }

func (transportSearch) Description() string {
	return "Найти варианты транспорта из локальной базы между городами на заданную дату."
}

func (transportSearch) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"from":{"type":"string","description":"Город отправления"},
			"to":{"type":"string","description":"Город назначения"},
			"departure_date":{"type":"string","description":"Дата отправления, YYYY-MM-DD"},
			"passengers":{"type":"integer","minimum":1,"description":"Количество пассажиров"}
		},
		"required":["from","to","departure_date","passengers"],
		"additionalProperties":false
	}`)
}

func (t transportSearch) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var input transportSearchInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return nil, fmt.Errorf("parse search_transport arguments: %w", err)
	}
	if strings.TrimSpace(input.From) == "" || strings.TrimSpace(input.To) == "" || strings.TrimSpace(input.DepartureDate) == "" {
		return nil, fmt.Errorf("from, to and departure_date are required")
	}
	if input.Passengers < 1 {
		return nil, fmt.Errorf("passengers must be positive")
	}
	if _, err := time.Parse("2006-01-02", input.DepartureDate); err != nil {
		return nil, fmt.Errorf("departure_date must use YYYY-MM-DD")
	}

	rows, err := t.db.QueryContext(ctx, `SELECT type, name, departure_time, duration_minutes,
		price_per_passenger, price_per_passenger * ? AS total_price
		FROM transport_options
		WHERE from_city = ? AND to_city = ?
		ORDER BY price_per_passenger`, input.Passengers, strings.TrimSpace(input.From), strings.TrimSpace(input.To))
	if err != nil {
		return nil, fmt.Errorf("query transport: %w", err)
	}
	defer rows.Close()

	options := make([]map[string]any, 0)
	for rows.Next() {
		var transportType, name, departureTime string
		var durationMinutes, pricePerPassenger, totalPrice int
		if err := rows.Scan(&transportType, &name, &departureTime, &durationMinutes, &pricePerPassenger, &totalPrice); err != nil {
			return nil, fmt.Errorf("read transport: %w", err)
		}
		options = append(options, map[string]any{
			"type": transportType, "name": name, "departure_time": departureTime,
			"duration_minutes": durationMinutes, "price_per_passenger": pricePerPassenger, "total_price": totalPrice,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read transport options: %w", err)
	}
	return map[string]any{
		"from":           input.From,
		"to":             input.To,
		"departure_date": input.DepartureDate,
		"passengers":     input.Passengers,
		"options":        options,
		"source":         "local_database",
	}, nil
}

type weather struct{}

type weatherInput struct {
	City string `json:"city"`
	Date string `json:"date"`
}

func NewWeather() Tool { return weather{} }

func (weather) Name() string { return "get_weather" }

func (weather) Description() string {
	return "Получить тестовый прогноз погоды в городе на указанную дату."
}

func (weather) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"city":{"type":"string","description":"Город"},
			"date":{"type":"string","description":"Дата прогноза, YYYY-MM-DD"}
		},
		"required":["city","date"],
		"additionalProperties":false
	}`)
}

func (weather) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var input weatherInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return nil, fmt.Errorf("parse get_weather arguments: %w", err)
	}
	if strings.TrimSpace(input.City) == "" || strings.TrimSpace(input.Date) == "" {
		return nil, fmt.Errorf("city and date are required")
	}
	return map[string]any{
		"city":          input.City,
		"date":          input.Date,
		"forecast":      "Переменная облачность",
		"temperature_c": 18,
		"source":        "mock",
	}, nil
}
