package relays

import (
	"context"
	"log"
	"rssnotes/internal/config"
	"rssnotes/internal/helpers"

	"github.com/fiatjaf/eventstore/badger"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/policies"
	"github.com/nbd-wtf/go-nostr"
)

var (
	db         = badger.BadgerBackend{}
	rly        = khatru.NewRelay()
	pool       *nostr.SimplePool
	seedRelays []string
	s          config.C
)

func RelayInit(cfg config.C) {
	s = cfg
	ctx := context.Background()
	pool = nostr.NewSimplePool(ctx)

	seedRelays = helpers.GetRelayListFromFile(cfg.SeedRelaysPath)
	if len(seedRelays) == 0 {
		log.Panic("[FATAL] 0 seed relays; need to set relays")
		return
	}

	//returned on the NIP-11 endpoint
	rly.Info.Name = cfg.RelayName
	rly.Info.PubKey = cfg.RelayPubkey
	rly.Info.Description = cfg.RelayDescription
	rly.Info.Contact = cfg.RelayContact
	rly.Info.Icon = cfg.RelayIcon

	db.Path = cfg.DatabasePath
	//os.MkdirAll(db.Path, 0755)
	if err := db.Init(); err != nil {
		log.Panicf("[FATAL] db init: %s", err)
		return
	}
	//defer db.Close()

	rly.StoreEvent = append(rly.StoreEvent, db.SaveEvent)
	rly.QueryEvents = append(rly.QueryEvents, db.QueryEvents)
	rly.CountEvents = append(rly.CountEvents, db.CountEvents)
	rly.DeleteEvent = append(rly.DeleteEvent, db.DeleteEvent)

	rly.RejectEvent = append(rly.RejectEvent,
		policyEventReadOnly,
	)

	rly.RejectFilter = append(rly.RejectFilter,
		policies.NoComplexFilters,
		policyFilterBookmark,
	)
}
