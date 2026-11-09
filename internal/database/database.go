package database

import (
	"fmt"
	"log"
	"time"

	"github.com/abundo/ergotime-server/internal/auth"
	"github.com/abundo/ergotime-server/internal/config"
	"github.com/abundo/ergotime-server/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect opens a PostgreSQL connection via GORM and runs migrations.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Warn
	if cfg.IsDebug() {
		logLevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := migrate(db); err != nil {
		return nil, err
	}

	if err := bootstrapAdmin(db, &cfg.Bootstrap); err != nil {
		return nil, err
	}

	return db, nil
}

func migrate(db *gorm.DB) error {
	// Clean cutover: drop legacy activity model and old project/report shapes.
	// Domain data is re-seeded via scripts/export_import.py; users are kept when possible.
	if err := dropLegacySchema(db); err != nil {
		return err
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Customer{},
		&models.Project{},
		&models.Report{},
	); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// GORM AutoMigrate does not reliably widen existing varchar → text.
	for _, stmt := range []string{
		`ALTER TABLE users ALTER COLUMN name TYPE text`,
		`ALTER TABLE users ALTER COLUMN full_name TYPE text`,
		`ALTER TABLE users ALTER COLUMN password TYPE text`,
		`ALTER TABLE customer ALTER COLUMN name TYPE text`,
		`ALTER TABLE customer ALTER COLUMN description TYPE text`,
		`ALTER TABLE project ALTER COLUMN name TYPE text`,
		`ALTER TABLE project ALTER COLUMN costcenter TYPE text`,
		`ALTER TABLE project ALTER COLUMN unit TYPE text`,
		`ALTER TABLE report ALTER COLUMN comment TYPE text`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			log.Printf("warning: column type migrate (%s): %v", stmt, err)
		}
	}

	// Project billing: unit label replaces hourly/km rates (columns may remain after model change).
	for _, col := range []string{"hourly_rate", "km_rate"} {
		var n int64
		_ = db.Raw(`
			SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'project' AND column_name = ?
		`, col).Scan(&n).Error
		if n > 0 {
			if err := db.Exec(fmt.Sprintf(`ALTER TABLE project DROP COLUMN IF EXISTS %s`, col)).Error; err != nil {
				log.Printf("warning: drop project.%s: %v", col, err)
			}
		}
	}
	// Ensure unit is populated on older rows.
	if err := db.Exec(`UPDATE project SET unit = 'hours' WHERE unit IS NULL OR unit = ''`).Error; err != nil {
		log.Printf("warning: project unit default: %v", err)
	}

	// Ensure report.seq has a usable sequence even if AutoMigrate did not
	// create a serial for a non-primary column.
	// setval cannot take 0 (PG sequences start at 1). Empty table: setval(1, false)
	// so the next nextval() returns 1. Non-empty: setval(max, true) → next is max+1.
	if err := db.Exec(`
		CREATE SEQUENCE IF NOT EXISTS report_seq_seq;
		SELECT setval(
			'report_seq_seq',
			GREATEST(COALESCE((SELECT MAX(seq) FROM report), 1), 1),
			(SELECT COALESCE(MAX(seq), 0) FROM report) > 0
		);
	`).Error; err != nil {
		log.Printf("warning: report seq sequence setup: %v", err)
	}

	// Idempotent client creates: unique client_uuid when set (partial index so
	// legacy rows with NULL remain valid).
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_report_client_uuid
		ON report (client_uuid)
		WHERE client_uuid IS NOT NULL AND client_uuid <> ''
	`).Error; err != nil {
		log.Printf("warning: report client_uuid unique index: %v", err)
	}

	return nil
}

// dropLegacySchema removes the old activity-based domain tables once.
// Triggered only when the legacy "activity" table still exists (or project
// lacks customer_id). Domain data is not preserved — re-seed via export_import.
// Users table is never dropped.
func dropLegacySchema(db *gorm.DB) error {
	needCutover := db.Migrator().HasTable("activity")
	if !needCutover {
		// Old report.activityid or project without customer_id
		var n int64
		if db.Migrator().HasTable("report") {
			_ = db.Raw(`
				SELECT COUNT(*) FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = 'report' AND column_name = 'activityid'
			`).Scan(&n).Error
			if n > 0 {
				needCutover = true
			}
		}
		if !needCutover && db.Migrator().HasTable("project") {
			_ = db.Raw(`
				SELECT COUNT(*) FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = 'project' AND column_name = 'customer_id'
			`).Scan(&n).Error
			// project exists but has no customer_id → old shape
			if n == 0 {
				needCutover = true
			}
		}
	}
	if !needCutover {
		return nil
	}

	log.Printf("migrate: legacy activity schema detected — dropping domain tables for clean cutover")
	for _, table := range []string{"timesheet_invoice", "report", "project", "activity", "customer"} {
		if db.Migrator().HasTable(table) {
			if err := db.Migrator().DropTable(table); err != nil {
				return fmt.Errorf("drop table %s: %w", table, err)
			}
			log.Printf("migrate: dropped table %s", table)
		}
	}
	return nil
}

func bootstrapAdmin(db *gorm.DB, boot *config.BootstrapConfig) error {
	if boot == nil || !boot.Enabled {
		return nil
	}

	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	hash, err := auth.HashPassword(boot.Password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}

	admin := models.User{
		Name:         boot.Username,
		PasswordHash: hash,
		Active:       true,
		IsAdmin:      true,
	}
	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("create bootstrap admin: %w", err)
	}
	log.Printf("bootstrap: created admin user %q (change password immediately)", boot.Username)
	return nil
}

// NextReportSeq returns the next monotonic sequence number for report sync.
func NextReportSeq(db *gorm.DB) (int64, error) {
	var seq int64
	// Prefer Postgres sequence when available
	err := db.Raw("SELECT nextval('report_seq_seq')").Scan(&seq).Error
	if err == nil && seq > 0 {
		return seq, nil
	}

	// Fallback: MAX(seq)+1 under a transaction lock
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw("SELECT COALESCE(MAX(seq), 0) + 1 FROM report FOR UPDATE").Scan(&seq).Error; err != nil {
			return err
		}
		return nil
	})
	return seq, err
}
