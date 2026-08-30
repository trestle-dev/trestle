package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "file:"+os.Args[1]+"?_pragma=journal_mode(WAL)")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Seed: one succeeded, one dead, one pending-with-attempts (retrying).
	rows := [][]any{
		{"job_success", "noop", `{}`, "succeeded", 1, 5, now, "", "", now, now},
		{"job_dead", "webhook", `{"targetId":"wh_1"}`, "dead", 5, 5, now, "", "boom", now, now},
		{"job_retry", "webhook", `{"targetId":"wh_1"}`, "pending", 1, 5, now, "", "", now, now},
	}
	for _, r := range rows {
		_, err := db.Exec("INSERT INTO _trestle_jobs(id,kind,payload_json,status,attempts,max_attempts,available_at,lease_until,last_error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", r...)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Println("seeded 3 jobs")
}
