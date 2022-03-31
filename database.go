package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"strings"

	_ "github.com/lib/pq"
)

func dbConnection() *sql.DB {
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", settings.Database.Host, settings.Database.Port, settings.Database.Username, settings.Database.Password, settings.Database.DBName))

	if err != nil {
		log.Panicln(err)
	}
	if err = db.Ping(); err != nil {
		log.Panicln(err)
	}

	return db
}

func runSQLFile(db *sql.DB, filename string) error {
	log.Printf("running %s on the database", filename)

	rawCmds, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("error reading file: %s\n", filename)
		return err
	}

	cmds := strings.Split(string(rawCmds), ";")
	for x := range cmds {
		cmd := strings.TrimSpace(cmds[x])

		if cmd == "" {
			continue
		}

		_, err = db.Exec(cmd)

		if err != nil {
			log.Printf("could not execute SQL: %s\n", cmd)
			return err
		}
	}

	return nil
}

func validateAndInitializeDB(db *sql.DB) {
	row := db.QueryRow("SELECT schema_version FROM kimmunity")
	var dbVersion uint
	err := row.Scan(&dbVersion)
	if err != nil {
		log.Println("Initializing the database...")
		err = runSQLFile(db, path.Join("sql", "initialize.sql"))
		if err != nil {
			log.Fatalln(err)
		}
	} else if dbVersion > majorVersion {
		log.Fatalln("The database is for a more recent version of the server!")
	} else if dbVersion < majorVersion {
		log.Println("Upgrading the database...")
		for dbVersion < majorVersion {
			err = runSQLFile(db, path.Join("sql", fmt.Sprintf("schema%dto%d.sql", dbVersion, dbVersion+1)))
			if err != nil {
				log.Fatalln(err)
			}
			dbVersion++
		}
	}
}
