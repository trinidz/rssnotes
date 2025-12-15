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
	relay      = khatru.NewRelay()
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
	relay.Info.Name = cfg.RelayName
	relay.Info.PubKey = cfg.RelayPubkey
	relay.Info.Description = cfg.RelayDescription
	relay.Info.Contact = cfg.RelayContact
	relay.Info.Icon = cfg.RelayIcon

	db.Path = cfg.DatabasePath
	//os.MkdirAll(db.Path, 0755)
	if err := db.Init(); err != nil {
		log.Panicf("[FATAL] db init: %s", err)
		return
	}
	defer db.Close()

	relay.StoreEvent = append(relay.StoreEvent, db.SaveEvent)
	relay.QueryEvents = append(relay.QueryEvents, db.QueryEvents)
	relay.CountEvents = append(relay.CountEvents, db.CountEvents)
	relay.DeleteEvent = append(relay.DeleteEvent, db.DeleteEvent)

	relay.RejectEvent = append(relay.RejectEvent,
		policyEventReadOnly,
	)

	relay.RejectFilter = append(relay.RejectFilter,
		policies.NoComplexFilters,
		policyFilterBookmark,
	)
}
