package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"travel-agent/internal/agent"
	"travel-agent/internal/database"
	"travel-agent/internal/llm"
	"travel-agent/internal/tools"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadEnvFile(".env"); err != nil {
		return err
	}

	client, err := llm.NewClient(llm.ConfigFromEnv())
	if err != nil {
		return err
	}
	dbPath := os.Getenv("TRAVEL_DB_PATH")
	if dbPath == "" {
		dbPath = "data/travel.db"
	}
	db, err := database.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	assistant := agent.New(client, []tools.Tool{
		tools.NewHotelSearch(db),
		tools.NewTransportSearch(db),
		tools.NewWeather(),
	})

	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Планировщик путешествий запущен. Для выхода нажмите Ctrl+D.")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		answer, err := assistant.Run(ctx, input)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Ошибка агента:", err)
			continue
		}
		fmt.Println(answer)
	}

	return scanner.Err()
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("открыть %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return fmt.Errorf("неверная строка %d в %s", lineNumber, path)
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '\'' && value[len(value)-1] == '\'') ||
				(value[0] == '"' && value[len(value)-1] == '"') {
				value = value[1 : len(value)-1]
			}
		}

		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("задать %s из %s: %w", name, path, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("прочитать %s: %w", path, err)
	}
	return nil
}
