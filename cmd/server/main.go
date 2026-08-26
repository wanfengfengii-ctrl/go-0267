// Command server is the runnable entry point for the incubation inspection
// backend. It opens the persistent SQLite store, seeds the reference catalog,
// recovers any open tasks and pending retries from the previous run, and
// serves the HTTP API.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/api"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "hatchseal.db", "path to the SQLite database file")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	svc := service.New(st)
	recover(ctx(), svc)

	srv := api.NewServer(svc)
	log.Printf("hatchseal incubation gate listening on %s (db=%s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// recover resumes open tasks and their unresolved device retries so a restart
// never loses in-flight work or releases resources early.
func recover(ctx context.Context, svc *service.Service) {
	tasks, err := svc.OpenTasks(ctx)
	if err != nil {
		log.Printf("recover: list open tasks: %v", err)
		return
	}
	for _, t := range tasks {
		retries, err := svc.PendingRetries(ctx, t.ID)
		if err != nil {
			log.Printf("recover: pending retries for %s: %v", t.ID, err)
			continue
		}
		log.Printf("recover: open task %s status=%s generation=%d pending_retries=%d",
			t.ID, t.Status, t.Generation, len(retries))
	}
}

func ctx() context.Context { return context.Background() }
