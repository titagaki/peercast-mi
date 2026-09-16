// audit-migrate applies the audit schema using separately supplied DDL credentials.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/titagaki/peercast-mi/internal/audit"
)

func main() {
	db, err := audit.OpenMySQL()
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit migration: database environment is incomplete")
		os.Exit(1)
	}
	defer db.DB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = db.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "audit migration failed; verify connectivity, dedicated database, DDL privileges and schema checksum")
		os.Exit(1)
	}
	fmt.Println("audit schema version 1 applied")
}
