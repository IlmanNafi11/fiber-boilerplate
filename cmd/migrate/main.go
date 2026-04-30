package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
)

func main() {
	allFlag := flag.Bool("all", false, "apply to all migrations (used with down)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	// Validate arguments that don't need DB access before loading config.
	switch command {
	case "force":
		if len(args) < 2 {
			log.Fatal("force requires a version argument (e.g., migrate force 1)")
		}
		if _, err := strconv.Atoi(args[1]); err != nil {
			log.Fatalf("force: invalid version %q: must be a number", args[1])
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	m, err := migrate.New(
		"file://db/migrations",
		cfg.Database.MigrateDSN(),
	)
	if err != nil {
		log.Fatalf("failed to create migrator: %v", err)
	}
	defer func() {
		if _, err := m.Close(); err != nil {
			log.Printf("warning: failed to close migrator: %v", err)
		}
	}()

	switch command {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migration up failed: %v", err)
		}
		if err == migrate.ErrNoChange {
			fmt.Println("no new migrations to apply")
		} else {
			fmt.Println("migrations applied successfully")
		}

	case "down":
		if *allFlag {
			if err := m.Drop(); err != nil {
				log.Fatalf("migration down -all failed: %v", err)
			}
			fmt.Println("all migrations rolled back successfully")
		} else {
			if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
				log.Fatalf("migration down failed: %v", err)
			}
			if err == migrate.ErrNoChange {
				fmt.Println("no migrations to rollback")
			} else {
				fmt.Println("migration rolled back successfully (1 step)")
			}
		}

	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			if err == migrate.ErrNilVersion {
				fmt.Println("no migrations applied yet")
				return
			}
			log.Fatalf("failed to get version: %v", err)
		}
		dirtyStatus := "clean"
		if dirty {
			dirtyStatus = "DIRTY"
		}
		fmt.Printf("current version: %d (%s)\n", version, dirtyStatus)

	case "force":
		forceVersion, _ := strconv.Atoi(args[1])
		if err := m.Force(forceVersion); err != nil {
			log.Fatalf("force version failed: %v", err)
		}
		fmt.Printf("forced version to %d\n", forceVersion)

	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("usage: migrate <up|down|version|force> [flags]")
	fmt.Println("")
	fmt.Println("commands:")
	fmt.Println("  up              apply all pending migrations")
	fmt.Println("  down            rollback last migration (use -all for all)")
	fmt.Println("  version         show current migration version")
	fmt.Println("  force <version> force migration version (dirty state recovery)")
}
