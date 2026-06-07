package server

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"strings"
)

//go:embed all:migrations
var migrationFiles embed.FS

func runMigrations(db DB, driver string) error {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("ler migrations: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		data, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("ler migration %s: %w", name, err)
		}
		sql := adaptSQL(string(data), driver)

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("executar migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
		log.Printf("migration aplicada: %s", name)
	}
	return nil
}

func adaptSQL(sql string, driver string) string {
	if driver == "sqlite" {
		return sql
	}
	var b strings.Builder
	n := 1
	for _, r := range sql {
		if r == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			n++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type DB interface {
	Begin() (Tx, error)
}

type Tx interface {
	Exec(query string, args ...any) (Result, error)
	Commit() error
	Rollback() error
}

type Result interface {
	RowsAffected() (int64, error)
}
