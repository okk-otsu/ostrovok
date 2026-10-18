package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := path + "?_foreign_keys=on&_busy_timeout=5000"
	if path == ":memory:" {
		dsn = "file:travel-agent-test?mode=memory&cache=shared&_foreign_keys=on"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open travel database: %w", err)
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if err := initialize(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func initialize(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS hotels (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			city TEXT NOT NULL COLLATE NOCASE,
			price_per_night INTEGER NOT NULL CHECK (price_per_night >= 0),
			rating REAL NOT NULL CHECK (rating BETWEEN 0 AND 5),
			max_guests INTEGER NOT NULL CHECK (max_guests > 0)
		)`,
		`CREATE TABLE IF NOT EXISTS transport_options (
			id INTEGER PRIMARY KEY,
			from_city TEXT NOT NULL COLLATE NOCASE,
			to_city TEXT NOT NULL COLLATE NOCASE,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			departure_time TEXT NOT NULL,
			duration_minutes INTEGER NOT NULL CHECK (duration_minutes > 0),
			price_per_passenger INTEGER NOT NULL CHECK (price_per_passenger >= 0)
		)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create travel database schema: %w", err)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin database seed: %w", err)
	}
	defer tx.Rollback()

	var hotelCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM hotels`).Scan(&hotelCount); err != nil {
		return fmt.Errorf("count hotels: %w", err)
	}
	hotels := []struct {
		name, city               string
		pricePerNight, maxGuests int
		rating                   float64
	}{
		{"Отель «Волжский берег»", "Казань", 6000, 4, 4.7},
		{"Апартаменты «Казанский кремль»", "Казань", 4250, 3, 4.5},
		{"Гостиница «Сити»", "Казань", 9000, 4, 4.8},
		{"Отель «Арбатский двор»", "Москва", 7500, 2, 4.6},
		{"Гостиница «Северная звезда»", "Санкт-Петербург", 6800, 4, 4.7},
		{"Отель «Морской бриз»", "Сочи", 8200, 3, 4.4},
	}
	if hotelCount == 0 {
		for _, hotel := range hotels {
			_, err = tx.ExecContext(ctx, `INSERT INTO hotels
				(name, city, price_per_night, rating, max_guests) VALUES (?, ?, ?, ?, ?)`,
				hotel.name, hotel.city, hotel.pricePerNight, hotel.rating, hotel.maxGuests)
			if err != nil {
				return fmt.Errorf("seed hotels: %w", err)
			}
		}
	}

	var transportCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transport_options`).Scan(&transportCount); err != nil {
		return fmt.Errorf("count transport options: %w", err)
	}
	if transportCount == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO transport_options
			(from_city, to_city, type, name, departure_time, duration_minutes, price_per_passenger)
			VALUES
			('Москва', 'Казань', 'поезд', 'Поезд №001', '20:08', 680, 2400),
			('Москва', 'Казань', 'автобус', 'Автобус «Восток»', '19:30', 780, 1800),
			('Казань', 'Москва', 'поезд', 'Поезд №002', '19:45', 690, 2500),
			('Казань', 'Москва', 'автобус', 'Автобус «Столица»', '18:20', 800, 1850),
			('Москва', 'Санкт-Петербург', 'поезд', 'Экспресс «Нева»', '07:00', 245, 3200),
			('Москва', 'Санкт-Петербург', 'автобус', 'Автобус «Балтика»', '22:00', 600, 2100),
			('Москва', 'Сочи', 'поезд', 'Поезд «Черноморец»', '14:20', 1440, 5200),
			('Москва', 'Сочи', 'самолёт', 'Рейс FA-104', '09:15', 230, 7800)`)
		if err != nil {
			return fmt.Errorf("seed transport options: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit database seed: %w", err)
	}
	return nil
}
