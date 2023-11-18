package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/miekg/dns"
	flag "github.com/spf13/pflag"
	"log"
	"net"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var dnsServer string

func main() {
	var dbUrl string

	flag.StringVar(&dbUrl, "db", "", "Database URL")
	flag.StringVar(&dnsServer, "dns", "", "DNS Server")
	flag.Parse()

	if len(dbUrl) == 0 {
		dbUrl = "postgresql:///blmonitor"
		log.Printf("connecting to %s", dbUrl)
	}

	if len(dnsServer) == 0 {
		cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			log.Fatalf("dns server required: %s", err)
		}
		dnsServer = cfg.Servers[0] + ":53"
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for range sigs {
			cancel()
			return
		}
	}()

	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatalln(err)
	}

	var lists []string
	// language=SQL
	err = db.QueryRow(ctx, `select array_agg(id order by id) from lists`).Scan(&lists)
	if err != nil {
		log.Fatalln(err)
	}
	if len(lists) == 0 {
		log.Fatalln("no lists configured")
	}
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		listen(ctx, db)
		wg.Done()
	}()

	for _, list := range lists {
		wg.Add(1)
		go func(list string) {
			defer wg.Done()
			err := monitor(ctx, db, list, dnsServer)
			if err != nil {
				log.Printf("monitoring %s failed with %s", list, err)
			}
		}(list)
	}
	wg.Wait()
}

func listen(ctx context.Context, db *pgxpool.Pool) {
	var lastChange time.Time
	err := db.AcquireFunc(ctx, func(conn *pgxpool.Conn) error {
		// language=SQL
		_, err := conn.Exec(ctx, `listen ip`)
		if err != nil {
			return fmt.Errorf("while listening for changes: %w", err)
		}
		// language=SQL
		err = conn.QueryRow(ctx, `select coalesce(max(stamp), 'epoch') from ips;`).Scan(&lastChange)
		if err != nil {
			return fmt.Errorf("while getting last change: %w", err)
		}
		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				return fmt.Errorf("while getting notification: %w", err)
			}
			log.Printf("new ip %s", notification.Payload)
			p := net.ParseIP(notification.Payload)
			if p == nil {
				log.Printf("failed to parse IP %s", notification.Payload)
				continue
			}
			newIp(ctx, db, p.String())
		}
	})
	if err != nil {
		log.Fatalf("failed while handling new ips: %s", err)
	}
}

func newIp(ctx context.Context, db *pgxpool.Pool, ip string) {
	// language=SQL
	rows, err := db.Query(ctx, `select id, stem from lists`)
	if err != nil {
		log.Fatalf("failed to get lists during delivery: %s", err)
	}
	defer rows.Close()
	for rows.Next() {
		var list, stem string
		err = rows.Scan(&list, &stem)
		if err != nil {
			log.Printf("failed to scan lists: %s", err)
			return
		}

		if !strings.HasPrefix(stem, ".") {
			stem = "." + stem
		}
		if !strings.HasSuffix(stem, ".") {
			stem = stem + "."
		}
		listed, msg, err := queryAandTXT(ctx, ip, stem, dnsServer)
		if err != nil {
			log.Printf("dns query error: %s", err)
			continue
		}
		err = recordChange(ctx, db, list, ip, listed, msg)
		if err != nil {
			log.Printf("failed to record initial change: %s", err)
		}
	}
}

func queryAandTXT(ctx context.Context, q, stem, dnsServer string) (bool, string, error) {
	c := new(dns.Client)
	octets := strings.Split(q, ".")
	slices.Reverse(octets)
	req := strings.Join(octets, ".") + stem
	m := new(dns.Msg)
	m.SetQuestion(req, dns.TypeA)
	//log.Printf("querying %s", req)
	in, _, err := c.ExchangeContext(ctx, m, dnsServer)
	if err != nil {
		return false, "", err
	}
	if len(in.Answer) == 0 {
		//log.Printf("no answer")
		return false, "", nil
	}
	t := new(dns.Msg)
	t.SetQuestion(req, dns.TypeTXT)
	in, _, err = c.ExchangeContext(ctx, t, dnsServer)
	if err != nil {
		return true, "", err
	}
	if len(in.Answer) > 0 {
		if txt, ok := in.Answer[0].(*dns.TXT); ok {
			return true, strings.Join(txt.Txt, ""), nil
		}
	}
	return true, "", nil
}

func recordChange(ctx context.Context, db *pgxpool.Pool, list, ip string, listed bool, msg string) error {
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// Update state
		// language=SQL
		tag, err := tx.Exec(ctx, `update state set lastip=$1, stamp=current_timestamp where id = $2`, ip, list)
		if err != nil {
			return fmt.Errorf("while updating state: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// language=SQL
			_, err := tx.Exec(ctx, `insert into state (lastip, stamp, id) values ($1, current_timestamp, $2)`, ip, list)
			if err != nil {
				return fmt.Errorf("while inserting state: %w", err)
			}
		}
		// language=SQL
		rows, err := tx.Query(ctx, `select listed from results where ip=$1 and list=$2 order by stamp desc limit 1`, ip, list)
		if err != nil {
			return fmt.Errorf("while getting previous listing state: %w", err)
		}
		resultChanged := true
		if rows.Next() {
			var oldListed bool
			err = rows.Scan(&oldListed)
			if err != nil {
				return fmt.Errorf("while scanning previous listing state: %w", err)
			}
			resultChanged = oldListed != listed
			for rows.Next() {
			}
		}
		if resultChanged {
			// language=SQL
			var customers []string
			// language=SQL
			err = tx.QueryRow(ctx, `select array_agg(distinct customer) from customer_ips where $1::inet << ip`, ip).Scan(&customers)
			if err != nil {
				return fmt.Errorf("while getting customers for IP: %w", err)
			}
			if len(customers) == 0 {
				customers = []string{"unknown"}
			}
			for _, cust := range customers {
				_, err = tx.Exec(ctx, `insert into results (ip, customer, list, txt, listed) VALUES ($1, $2, $3, $4, $5)`,
					ip, cust, list, msg, listed)
				if err != nil {
					return fmt.Errorf("while updating listing state: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("while recording result for %s: %w", ip, err)
	}
	return nil
}

func monitor(ctx context.Context, db *pgxpool.Pool, list string, dnsServer string) error {
	var lastip string
	var stamp time.Time

	// language=SQL
	err := db.QueryRow(ctx, `select lastip::text, stamp from state where id=$1`, list).Scan(&lastip, &stamp)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("while getting initial state: %w", err)
	}
	for {
		log.Printf("starting scan for %s", list)
		scanStart := time.Now()
		var listType, stem, testpos, testneg string
		var throttle, period int
		// language=SQL
		err := db.QueryRow(ctx, `select type, stem, testpos, testneg, throttle, period from lists where id=$1`, list).Scan(&listType, &stem, &testpos, &testneg, &throttle, &period)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(stem, ".") {
			stem = "." + stem
		}
		if !strings.HasSuffix(stem, ".") {
			stem = stem + "."
		}

		if len(testpos) > 0 {
			ok, _, err := queryAandTXT(ctx, testpos, stem, dnsServer)
			if err != nil {
				return fmt.Errorf("while querying positive test: %w", err)
			}
			if !ok {
				return fmt.Errorf("positive test failed")
			}
		}

		if len(testneg) > 0 {
			ok, _, err := queryAandTXT(ctx, testneg, stem, dnsServer)
			if err != nil {
				return fmt.Errorf("while querying negative test: %w", err)
			}
			if ok {
				return fmt.Errorf("negative test failed")
			}
		}

		var customers []string
		// language=SQL
		err = db.QueryRow(ctx, `select array_agg(id) from customers where lists is null or $1 = ANY(lists)`, list).Scan(&customers)
		if err != nil {
			return fmt.Errorf("while getting customers: %w", err)
		}

		//ipCustomer := map[string][]string{}
		//// language=SQL
		//rows, err := db.Query(ctx, `select ip::text, customer from customer_ips where customer = ANY($1::text[])`, customers)
		//if err != nil {
		//	return fmt.Errorf("while fetching ips: %w", err)
		//}
		//for rows.Next() {
		//	var ip, cust string
		//	err = rows.Scan(&ip, &cust)
		//	if err != nil {
		//		return fmt.Errorf("while scanning ips: %w", err)
		//	}
		//	ipCustomer[ip] = append(ipCustomer[ip], cust)
		//}
		var ips []string
		// language=SQL
		err = db.QueryRow(ctx, `select array_agg(ip::text) from ips`).Scan(&ips)
		if err != nil {
			return fmt.Errorf("while reading ips: %w", err)
		}
		sort.Strings(ips)
		for _, ip := range ips {
			if lastip > ip {
				continue
			}
			listed, msg, err := queryAandTXT(ctx, ip, stem, dnsServer)
			if err != nil {
				log.Printf("Error querying %s for %s: %s", list, ip, err.Error())
				continue
			}
			log.Printf("  %s %s -> %v %s", list, ip, listed, msg)
			err = recordChange(ctx, db, list, ip, listed, msg)
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(throttle) * time.Second):
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		log.Printf("finished scan for %s", list)
		lastip = ""
		elapsed := time.Since(scanStart)
		if elapsed < time.Duration(period)*time.Second {
			log.Printf("%s waiting until %s", list, time.Now().Add(time.Duration(period)*time.Second-elapsed).String())
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(period)*time.Second - elapsed):
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
