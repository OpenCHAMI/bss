// Copyright © 2023 Triad National Security, LLC. All rights reserved.
//
// This program was produced under U.S. Government contract 89233218CNA000001 for
// Los Alamos National Laboratory (LANL), which is operated by Triad National
// Security, LLC for the U.S. Department of Energy/National Nuclear Security
// Administration. All rights in the program are reserved by Triad National
// Security, LLC, and the U.S. Department of Energy/National Nuclear Security
// Administration. The Government is granted for itself and others acting on its
// behalf a nonexclusive, paid-up, irrevocable worldwide license in this material
// to reproduce, prepare derivative works, distribute copies to the public,
// perform publicly and display publicly, and to permit others to do so.

/*
 * Boot Script Server Initializer
 *
 * bss-init initializes a PostgreSQL database with the schema required to run
 * BSS using a PostgreSQL backend.
 *
 */

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/golang-migrate/migrate/v4"
	db "github.com/golang-migrate/migrate/v4/database"
	pg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

const (
	APP_VERSION    = "1"
	SCHEMA_VERSION = 1
	SCHEMA_STEPS   = 2
)

var (
	sqlHost          = "localhost"
	sqlPort          = uint(5432)
	sqlInsecure      = false
	sqlFresh         = false
	sqlDbName        = "bssdb"
	sqlDbOpts        = ""
	sqlUser          = "bssuser"
	sqlPass          = "bssuser"
	sqlRetryInterval = uint64(5)
	sqlRetryCount    = uint64(10)
	sqlMigrationDir  = "/migrations"
	printVersion     = false
	migrateStep      = uint(SCHEMA_STEPS)
	forceStep        = -1
	lg               = log.New(os.Stdout, "", log.Lshortfile|log.LstdFlags|log.Lmicroseconds)
	bssdb            *sql.DB
)

func parseEnv(evar string, v interface{}) (ret error) {
	if val := os.Getenv(evar); val != "" {
		switch vp := v.(type) {
		case *int:
			var temp int64
			temp, ret = strconv.ParseInt(val, 0, 64)
			if ret == nil {
				*vp = int(temp)
			}
		case *uint:
			var temp uint64
			temp, ret = strconv.ParseUint(val, 0, 64)
			if ret == nil {
				*vp = uint(temp)
			}
		case *string:
			*vp = val
		case *bool:
			switch strings.ToLower(val) {
			case "0", "off", "no", "false":
				*vp = false
			case "1", "on", "yes", "true":
				*vp = true
			default:
				ret = fmt.Errorf("Unrecognized bool value: '%s'", val)
			}
		case *[]string:
			*vp = strings.Split(val, ",")
		default:
			ret = fmt.Errorf("Invalid type for receiving ENV variable value %T", v)
		}
	}
	return
}

func parseEnvVars() error {
	var (
		err      error = nil
		parseErr error
		errList  []error
	)
	parseErr = parseEnv("BSS_INSECURE", &sqlInsecure)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_INSECURE: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBHOST", &sqlHost)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBHOST: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBPORT", &sqlPort)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBPORT: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBNAME", &sqlDbName)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBNAME: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBOPTS", &sqlDbOpts)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBOPTS: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBUSER", &sqlUser)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBUSER: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBPASS", &sqlPass)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBPASS: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBSTEP", &migrateStep)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBSTEP: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBFORCESTEP", &forceStep)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBFORCESTEP: %q", parseErr))
	}
	parseErr = parseEnv("BSS_MIGRATIONDIR", &sqlMigrationDir)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_MIGRATIONDIR: %q", parseErr))
	}
	parseErr = parseEnv("BSS_FRESH", &sqlFresh)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_FRESH: %q", parseErr))
	}
	parseErr = parseEnv("BSS_SQL_RETRY_WAIT", &sqlRetryInterval)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_SQL_RETRY_WAIT: %q", parseErr))
	}
	parseErr = parseEnv("BSS_SQL_RETRY_COUNT", &sqlRetryCount)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_SQL_RETRY_COUNT: %q", parseErr))
	}

	if len(errList) > 0 {
		err = fmt.Errorf("Error(s) parsing environment variables: %v", errList)
	}

	return err
}

func parseCmdLine() {
	flag.StringVar(&sqlHost, "postgres-host", sqlHost, "(BSS_DBHOST) Postgres host as IP address or name")
	flag.StringVar(&sqlUser, "postgres-user", sqlUser, "(BSS_DBUSER) Postgres username")
	flag.StringVar(&sqlPass, "postgres-password", sqlPass, "(BSS_DBPASS) Postgres password")
	flag.StringVar(&sqlDbName, "postgres-dbname", sqlDbName, "(BSS_DBNAME) Postgres database name")
	flag.StringVar(&sqlDbOpts, "postgres-dbopts", sqlDbOpts, "(BSS_DBOPTS) Postgres database options")
	flag.StringVar(&sqlMigrationDir, "postgres-migrations", sqlMigrationDir, "(BSS_MIGRATIONDIR) Postgres migrations directory path")
	flag.IntVar(&forceStep, "force-step", forceStep, "(BSS_DBFORCESTEP) Migration step number to force migrate to before performing migration")
	flag.UintVar(&sqlPort, "postgres-port", sqlPort, "(BSS_DBPORT) Postgres port")
	flag.UintVar(&migrateStep, "step", migrateStep, "(BSS_DBSTEP) Migration step number to migrate to")
	flag.Uint64Var(&sqlRetryCount, "postgres-retry-count", sqlRetryCount, "(BSS_SQL_RETRY_COUNT) Number of times to retry connecting to Postgres database before giving up")
	flag.Uint64Var(&sqlRetryInterval, "postgres-retry-interval", sqlRetryInterval, "(BSS_SQL_RETRY_WAIT) Seconds to wait between retrying connection to Postgres")
	flag.BoolVar(&sqlInsecure, "postgres-insecure", sqlInsecure, "(BSS_INSECURE) Don't enforce certificate authority for Postgres")
	flag.BoolVar(&sqlFresh, "fresh", sqlFresh, "(BSS_FRESH) Revert all schemas before migration (drops all BSS-related tables)")
	flag.BoolVar(&printVersion, "version", printVersion, "Print version and exit")
	flag.Parse()
}

func sqlOpen(host string, port uint, dbName, user, password string, ssl bool, extraDbOpts string, retryCount, retryWait uint64) (*sql.DB, error) {
	var (
		err     error
		bddb    *sql.DB
		sslmode string
		ix      = uint64(1)
	)
	if ssl {
		sslmode = "verify-full"
	} else {
		sslmode = "disable"
	}
	connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s", host, port, dbName, user, password, sslmode)
	if extraDbOpts != "" {
		connStr += " " + extraDbOpts
	}
	lg.Println(connStr)

	// Connect to postgres, looping every retryWait seconds up to retryCount times.
	for ; ix <= retryCount; ix++ {
		lg.Printf("Attempting connection to Postgres at %s:%d (attempt %d)", host, port, ix)
		bddb, err = sql.Open("postgres", connStr)
		if err != nil {
			lg.Printf("ERROR: failed to open connection to Postgres at %s:%d (attempt %d, retrying in %d seconds): %v\n", host, port, ix, retryWait, err)
		} else {
			break
		}

		time.Sleep(time.Duration(retryWait) * time.Second)
	}
	if ix > retryCount {
		err = fmt.Errorf("Postgres connection attempts exhausted (%d).", retryCount)
	} else {
		lg.Printf("Initialized connection to Postgres database at %s:%d", host, port)
	}

	// Ping postgres, looping every retryWait seconds up to retryCount times.
	for ; ix <= retryCount; ix++ {
		lg.Printf("Attempting to ping Postgres connection at %s:%d (attempt %d)", host, port, ix)
		err = bddb.Ping()
		if err != nil {
			lg.Printf("ERROR: failed to ping Postgres at %s:%d (attempt %d, retrying in %d seconds): %v\n", host, port, ix, retryWait, err)
		} else {
			break
		}

		time.Sleep(time.Duration(retryWait) * time.Second)
	}
	if ix > retryCount {
		err = fmt.Errorf("Postgres ping attempts exhausted (%d).", retryCount)
	} else {
		lg.Printf("Pinged Postgres database at %s:%d", host, port)
	}

	return bddb, err
}

func sqlClose() {
	err := bssdb.Close()
	if err != nil {
		lg.Fatalf("ERROR: Attempt to close connection to Postgres failed: %v", err)
	}
}

func main() {
	var err error

	err = parseEnvVars()
	if err != nil {
		lg.Println(err)
		lg.Println("WARNING: Ignoring environment variables with errors.")
	}

	parseCmdLine()

	if printVersion {
		fmt.Printf("Version: %s, Schema Version: %d\n", APP_VERSION, SCHEMA_VERSION)
		os.Exit(0)
	}

	lg.Printf("bss-init: Starting...")
	lg.Printf("bss-init: Version: %s, Schema Version: %d, Steps: %d, Desired Step: %d",
		APP_VERSION, SCHEMA_VERSION, SCHEMA_STEPS, migrateStep)

	// Check vars.
	if forceStep < 0 || forceStep > SCHEMA_STEPS {
		if forceStep != -1 {
			// A negative value was passed (-1 is noop).
			lg.Fatalf("force-step value %d out of range, should be between (inclusive) 0 and %d", forceStep, SCHEMA_STEPS)
		}
	}

	if sqlInsecure {
		lg.Printf("WARNING: Using insecure connection to postgres.")
	}

	// Open connection to postgres.
	bssdb, err = sqlOpen(sqlHost, sqlPort, sqlDbName, sqlUser, sqlPass, !sqlInsecure, sqlDbOpts, sqlRetryCount, sqlRetryInterval)
	if err != nil {
		lg.Fatalf("ERROR: Access to Postgres database at %s:%d failed: %v\n", sqlHost, sqlPort, err)
	}
	lg.Printf("Successfully connected to Postgres at %s:%d", sqlHost, sqlPort)
	defer sqlClose()

	// Create instance of postgres driver to be used in migration instance creation.
	var pgdriver db.Driver
	pgdriver, err = pg.WithInstance(bssdb, &pg.Config{})
	if err != nil {
		lg.Fatalf("ERROR: Creating postgres driver failed: %v", err)
	}
	lg.Printf("Successfully created postgres driver")

	// Create migration instance pointing to migrations directory.
	var m *migrate.Migrate
	m, err = migrate.NewWithDatabaseInstance(
		"file://"+sqlMigrationDir,
		"postgres",
		pgdriver)
	if err != nil {
		lg.Fatalf("ERROR: Failed to create migration: %v", err)
	} else if m == nil {
		lg.Fatalf("ERROR: Failed to create migration: nil pointer")
	}
	defer m.Close()
	lg.Printf("Successfully created migration instance")

	// If --fresh specified, perform all down migrations (drop tables).
	if sqlFresh {
		err = m.Down()
		if err != nil {
			lg.Fatalf("ERROR: migration.Down() failed: %v", err)
		}
		lg.Printf("migration.Down() succeeded")
	}

	// Force specific migration step if specified (doesn't matter if dirty, since
	// the step is user-specified).
	if forceStep >= 0 {
		err = m.Force(forceStep)
		if err != nil {
			lg.Fatalf("ERROR: migration.Force(%d) failed: %v", forceStep, err)
		}
		lg.Printf("migration.Force(%d) succeeded", forceStep)
	}

	// Check if "dirty" (version is > 0), force current version to clear dirty flag
	// if dirty flag is set.
	var (
		version   uint
		noVersion = false
		dirty     = false
	)
	version, dirty, err = m.Version()
	if err == migrate.ErrNilVersion {
		lg.Printf("No migrations have been applied yet (version=%d)", version)
		noVersion = true
	} else if err != nil {
		lg.Fatalf("ERROR: Migration failed unexpectedly: %v", err)
	} else {
		lg.Printf("Migration at step %d (dirty=%t)", version, dirty)
	}
	if dirty && forceStep < 0 {
		lg.Printf("Migration is dirty and no --force-step specified, forcing current version")
		// Migration is dirty and no version to force was specified.
		// Force the current version to clear the dirty flag.
		// This situation should generally be avoided.
		err = m.Force(int(version))
		if err != nil {
			lg.Fatalf("ERROR: Forcing current version to clear dirty flag failed: %v", err)
		}
		lg.Printf("Forcing current version to clear dirty flag succeeded")
	}

	if noVersion {
		// Fresh installation, migrate from start to finish.
		lg.Printf("Migration: Initial install, calling Up()")
		err = m.Up()
		if err == migrate.ErrNoChange {
			lg.Printf("Migration: Up(): No changes applied (none needed)")
		} else if err != nil {
			lg.Fatalf("ERROR: Migration: Up() failed: %v", err)
		} else {
			lg.Printf("Migration: Up() succeeded")
		}
	} else if version != migrateStep {
		// Current version does not match user-specified version.
		// Migrate up or down from current version to target version.
		if version < migrateStep {
			lg.Printf("Migration: DB at version %d, target version %d; upgrading", version, migrateStep)
		} else {
			lg.Printf("Migration: DB at version %d, target version %d; downgrading", version, migrateStep)
		}
		err = m.Migrate(migrateStep)
		if err == migrate.ErrNoChange {
			lg.Printf("Migration: No changes applied (none needed)")
		} else if err != nil {
			lg.Fatalf("ERROR: Migration failed: %v", err)
		} else {
			lg.Printf("Migration succeeded")
		}
	} else {
		lg.Printf("Migration: Already at target version (%d), nothing to do", version)
	}
	version = 0
	dirty = false
	lg.Printf("Checking resulting migration version")
	version, dirty, err = m.Version()
	if err == migrate.ErrNilVersion {
		lg.Printf("WARNING: No version after migration")
	} else if err != nil {
		lg.Fatalf("ERROR: migration.Version() failed: %v", err)
	} else {
		lg.Printf("Migration at version %d (dirty=%t)", version, dirty)
	}
}
