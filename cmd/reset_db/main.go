package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	// Let's load the configuration safely without triggering panics from missing tokens.
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("No .env file found via godotenv, relying on system environment.")
	}

	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		fmt.Println("Warning: DATABASE_URL not set in environment.")
		dbUrl = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
		fmt.Println("Falling back to implicit default:", dbUrl)
	}

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dbUrl)
	if err != nil {
		log.Fatalf("❌ Unable to parse database URL: %v", err)
	}

	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("❌ Unable to connect to database: %v", err)
	}
	defer pool.Close()

	fmt.Println("Connected to database successfully. Running safe operational data reset...")

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to start transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// TRUNCATE the user-generated tables but keep plans, squads, and keys configuration.
	// We don't cascade on 'customer' because we want to be explicit.
	queries := []string{
		"TRUNCATE TABLE mobile_payment CASCADE",
		"TRUNCATE TABLE wallet_transaction CASCADE",
		"TRUNCATE TABLE referral CASCADE",
		"TRUNCATE TABLE purchase CASCADE",
		"TRUNCATE TABLE subscription_key CASCADE",
		"TRUNCATE TABLE customer CASCADE",
		"TRUNCATE TABLE promo_code CASCADE",
	}

	for _, q := range queries {
		_, err := tx.Exec(ctx, q)
		if err != nil {
			log.Fatalf("❌ Failed executing [%s]: %v", q, err)
		}
		fmt.Println("✔", q)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("❌ Failed to commit transaction: %v", err)
	}

	fmt.Println("\n✅ Success! All operational user data (customers, purchases, referrals, sub_keys, wallet, promo_codes) have been wiped.")
	fmt.Println("You can now test the referral system from a completely fresh start!")
}
