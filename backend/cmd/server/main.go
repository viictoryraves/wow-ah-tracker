package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/api"
	"github.com/manuel/wow-ah-tracker/internal/blizzard"
	"github.com/manuel/wow-ah-tracker/internal/config"
	"github.com/manuel/wow-ah-tracker/internal/ingest"
	"github.com/manuel/wow-ah-tracker/internal/notify"
	"github.com/manuel/wow-ah-tracker/internal/rates"
	"github.com/manuel/wow-ah-tracker/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	ing := ingest.New(blizzard.New(cfg.ClientID, cfg.ClientSecret, cfg.Region), st, cfg.Namespace(), cfg.StaticNamespace())
	realms, err := ing.ResolveRealms(ctx, cfg.RulesetIDs)
	if err != nil {
		log.Fatalf("resolve rulesets: %v", err)
	}
	var apiRealms []api.Realm
	for _, r := range realms {
		var houses []api.House
		for _, h := range r.Houses {
			houses = append(houses, api.House{ID: h.ID, Faction: h.Faction})
		}
		apiRealms = append(apiRealms, api.Realm{ID: r.ID, Name: r.Name, Houses: houses})
		log.Printf("tracking WoW Forever %s (%s): %d auction houses", r.Name, cfg.Region, len(houses))
	}

	// A macOS notification when new prices reach the database: one per hourly burst, not one per house.
	if cfg.Notifications && runtime.GOOS == "darwin" {
		notifier := notify.MacOS{}
		batch := notify.NewBatcher(notify.Window, func(u ingest.Update) {
			title, body := notify.Format(u)
			if err := notifier.Notify(title, body); err != nil {
				log.Printf("notification failed: %v", err)
				return
			}
			log.Printf("notification sent: %s | %s", title, body)
		})
		ing.OnUpdate = batch.Add
		defer batch.Flush()
	}

	go ing.Run(ctx, realms, cfg.PollInterval, cfg.RetentionDays)

	// Read the cut and deposit rates from the client tables now and then every day; the
	// configured percentages stay the fallback.
	rules := api.Rules{CutPercent: cfg.CutPercent, BlackwaterCutPercent: cfg.BlackwaterCutPercent}
	if cfg.LiveRates {
		table := rates.New(rates.DefaultBaseURL)
		rules.Rates = table
		go table.Run(ctx, cfg.RatesInterval)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: api.Handler(st, apiRealms, cfg.RetentionDays, rules)}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
