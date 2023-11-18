package main

import (
	"bufio"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	flag "github.com/spf13/pflag"
	"log"
	"net"
	"os"
	"strings"
)

func main() {
	var dbUrl string

	flag.StringVar(&dbUrl, "db", "", "Database URL")
	flag.Parse()

	if len(dbUrl) == 0 {
		dbUrl = "postgresql://blmonitor@/blmonitor"
	}

	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatalln(err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		ip := net.ParseIP(line)
		if ip != nil {
			// language=SQL
			_, err := db.Exec(context.Background(), `insert into ips (ip) values ($1) on conflict do nothing`, ip.String())
			if err != nil {
				log.Fatalf("failed to insert ip %s: %s", ip, err)
			}
		}
	}
}
