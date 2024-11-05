package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

const KIND_BOOKMARKS int = 10003 //NIP-51

// gets notes from the local db
func getLocalEvents(localFilter nostr.Filter) ([]*nostr.Event, error) {
	ctx := context.TODO()

	ch, err := db.QueryEvents(ctx, localFilter)
	if err != nil {
		log.Printf("[ERROR] QueryEvents %s", err)
		return nil, err
	}

	events := make([]*nostr.Event, 0)

	for evt := range ch {
		events = append(events, evt)
	}

	if len(events) == 0 {
		log.Print("[DEBUG] no events found")
		return []*nostr.Event{}, nil
	}

	//sort events from oldest [len-1] to most current [0]
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt > events[j].CreatedAt
	})

	//log.Printf("[DEBUG] %v events found", len(events))

	return events, nil
}

// get kind-0 metadata event of a pubkey
func getLocalMetadataEvent(pubkey string) (KindProfileMetadata, nostr.Event, error) {

	metaDataFilter := nostr.Filter{
		Kinds:   []int{nostr.KindProfileMetadata},
		Authors: []string{pubkey},
	}

	metaData, err := getLocalEvents(metaDataFilter)
	if err != nil {
		log.Print("[ERROR]", err)
		return KindProfileMetadata{}, nostr.Event{}, err
	}

	if len(metaData) == 0 {
		log.Printf("[DEBUG] no profile data found for pubkey %s", pubkey)
		return KindProfileMetadata{}, nostr.Event{}, nil
	}

	profileData := KindProfileMetadata{}
	if err := json.Unmarshal([]byte(metaData[0].Content), &profileData); err != nil {
		log.Print("[ERROR]", err)
		return KindProfileMetadata{}, nostr.Event{}, err
	}

	return profileData, *metaData[0], nil
}

func getRemoteFollows(pubkeyHex string) nostr.Tags {
	var outputFollows []nostr.Tag
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	pubKeyAlreadyExists := false
	defer cancel()

	filters := []nostr.Filter{{
		Authors: []string{pubkeyHex},
		Kinds:   []int{nostr.KindFollowList},
	}}

	relayEvents := make([]nostr.RelayEvent, 0)
	for ev := range pool.SubManyEose(timeoutCtx, seedRelays, filters) {
		relayEvents = append(relayEvents, ev)
	}

	if len(relayEvents) > 0 {
		sort.SliceStable(relayEvents, func(i, j int) bool {
			return relayEvents[i].CreatedAt > relayEvents[j].CreatedAt
		})

		log.Printf("[DEBUG] kind-3 found in %s with createdat %s with %d follows", relayEvents[0].Relay.URL, relayEvents[0].Event.CreatedAt.Time().String(), len(relayEvents[0].Event.Tags.GetAll([]string{"p"})))

		for _, remoteFollow := range relayEvents[0].Event.Tags.GetAll([]string{"p"}) {
			for _, outputFollow := range outputFollows {
				if outputFollow.Value() == remoteFollow.Value() || len(remoteFollow.Value()) != 64 || len(remoteFollow) != 2 {
					pubKeyAlreadyExists = true
					break
				}
			}
			if !pubKeyAlreadyExists {
				outputFollows = append(outputFollows, remoteFollow)
			}
			pubKeyAlreadyExists = false
		}
	} else {
		log.Print("[DEBUG] no remote follows found")
		return nil
	}

	return outputFollows
}

func getLocalFollows() nostr.Tags {
	var localFollows []nostr.Tag

	savedEnts, err := getSavedEntities()
	if err != nil {
		log.Printf("[ERROR] Can not get local follows %s", err)
		return nil
	}

	for _, savedEnt := range savedEnts {
		localFollows = append(localFollows, nostr.Tag{"p", savedEnt.PublicKey})
	}

	return localFollows
}

func getUniqueFollows(followListA nostr.Tags, followListB nostr.Tags) nostr.Tags {
	var uniqueFollows []nostr.Tag
	badPubkey := false

	uniqueFollows = append(uniqueFollows, followListB...)

	for _, followA := range followListA {
		for _, followB := range followListB {
			if len(followA) != 2 || len(followB) != 2 ||
				followA.Key() != "p" || followB.Key() != "p" ||
				len(followA.Value()) != 64 || len(followB.Value()) != 64 ||
				followA.Value() == followB.Value() {
				badPubkey = true
				break
			}
		}
		if !badPubkey {
			uniqueFollows = append(uniqueFollows, nostr.Tag{"p", followA.Value()})
		}
		badPubkey = false
	}
	return uniqueFollows
}

func deleteRemoteFollow(pubkeyHex string) nostr.Tags {
	remoteFollows := getRemoteFollows(s.RelayPubkey)

	for i, remoteFollow := range remoteFollows {
		if remoteFollow.Value() == pubkeyHex {
			copy(remoteFollows[i:], remoteFollows[i+1:])
			remoteFollows[len(remoteFollows)-1] = nostr.Tag{}
			remoteFollows = remoteFollows[:len(remoteFollows)-1]

			return remoteFollows
		}
	}

	return nil
}

func getRelayListFromFile(filePath string) []string {
	file, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read file: %s", err)
	}

	var relayList []string
	if err := json.Unmarshal(file, &relayList); err != nil {
		log.Fatalf("Failed to parse JSON: %s", err)
	}

	for i, relay := range relayList {
		relayList[i] = "wss://" + strings.TrimSpace(relay)
	}
	return relayList
}

// TRUE if a rsslay feedUrl already exists in bookmark note
func feedExists(pubkeyHex, privKeyHex, feedUrl string) (bool, error) {
	var bookMarkTags nostr.Tags

	var bookmarkFilter nostr.Filter = nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	bookMarkEvts, err := getLocalEvents(bookmarkFilter)
	if err != nil {
		log.Printf("[ERROR] GetLocalEvent %s", err)
		return false, err
	}

	if len(bookMarkEvts) > 0 {
		bookMarkTags = bookMarkEvts[0].Tags.GetAll([]string{s.RsslayTagKey})
		for _, tag := range bookMarkTags {
			if strings.Contains(tag.Value(), pubkeyHex) || strings.Contains(tag.Value(), privKeyHex) || strings.Contains(tag.Value(), feedUrl) {
				log.Printf("[DEBUG] feedUrl %s already exists", feedUrl)
				return true, nil
			}
		}
	}

	log.Printf("[DEBUG] feed %s does not exist", feedUrl)
	return false, nil
}

// add a feed entity to the bookmark event
func addEntityToBookmarkEvent(entitiesToAdd []Entity) error {
	var bookMarkTags nostr.Tags

	var bookmarkFilter nostr.Filter = nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	bookMarkEvts, err := getLocalEvents(bookmarkFilter)
	if err != nil {
		log.Printf("[ERROR] GetLocalEvent %s", err)
		return err
	}

	if len(bookMarkEvts) > 0 {
		bookMarkTags = bookMarkEvts[0].Tags.GetAll([]string{s.RsslayTagKey})
	}

	for _, ent := range entitiesToAdd {
		entityByteArr, err := json.Marshal(ent)
		if err == nil {
			bookMarkTags = append(bookMarkTags, nostr.Tag{s.RsslayTagKey, string(entityByteArr)})
		} else {
			log.Printf("[ERROR] %s", err)
		}
	}

	evt := nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      KIND_BOOKMARKS,
		Content:   "{rsslay, pubkey, privkey, url, last_update}",
		Tags:      bookMarkTags,
	}

	if err := evt.Sign(s.RelayPrivkey); err != nil {
		log.Printf("[ERROR] signing event %s", err)
		return err
	}

	// to store these events you must call the store functions manually
	for _, store := range relay.StoreEvent {
		store(context.TODO(), &evt)
	}

	log.Printf("[DEBUG] bookmark event %s stored", evt.ID)
	return nil
}

// update an existing feed entity 'last' property in the bookmark event
func updateEntityInBookmarkEvent(pubKeyHex string, lastUpdate int64) error {
	var bookMarkTags nostr.Tags
	var rsslayEntity Entity

	var bookmarkFilter nostr.Filter = nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	bookMarkEvts, err := getLocalEvents(bookmarkFilter)
	if err != nil {
		log.Printf("[ERROR] GetLocalEvent %s", err)
		return err
	}

	if len(bookMarkEvts) > 0 {
		bookMarkTags = bookMarkEvts[0].Tags.GetAll([]string{s.RsslayTagKey})
		for i, tag := range bookMarkTags {
			if strings.Contains(tag.Value(), pubKeyHex) {
				if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
					log.Printf("[ERROR] %s", err)
				}

				copy(bookMarkTags[i:], bookMarkTags[i+1:])
				bookMarkTags[len(bookMarkTags)-1] = nostr.Tag{}
				bookMarkTags = bookMarkTags[:len(bookMarkTags)-1]

				rsslayEntity.LastUpdate = lastUpdate

				jsonentArr, err := json.Marshal(rsslayEntity)
				if err != nil {
					log.Printf("[ERROR] %s", err)
				}

				bookMarkTags = append(bookMarkTags, nostr.Tag{s.RsslayTagKey, string(jsonentArr)})

				evt := nostr.Event{
					CreatedAt: nostr.Now(),
					Kind:      KIND_BOOKMARKS,
					Content:   "{rsslay, pubkey, privkey, url, last_update}",
					Tags:      bookMarkTags,
				}

				if err := evt.Sign(s.RelayPrivkey); err != nil {
					log.Printf("[ERROR] signing event %s", err)
					return err
				}

				for _, store := range relay.StoreEvent {
					store(context.TODO(), &evt)
				}

				log.Printf("[DEBUG] entity %s last update %d in event ID %s", rsslayEntity.URL, rsslayEntity.LastUpdate, evt.ID)
				break
			}
		}
	} else {
		log.Printf("[DEBUG] bookmark event not found")
	}

	return nil
}

// delete a feed entity from a local bookmark event
func deleteEntityInBookmarkEvent(pubKeyORfeedUrl string) error {
	var bookMarkTags nostr.Tags
	var rsslayEntity Entity

	var bookmarkFilter nostr.Filter = nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	bookMarkEvts, err := getLocalEvents(bookmarkFilter)
	if err != nil {
		log.Printf("[ERROR] GetLocalEvent %s", err)
		return err
	}

	if len(bookMarkEvts) > 0 {
		bookMarkTags = bookMarkEvts[0].Tags.GetAll([]string{s.RsslayTagKey})
		for i, tag := range bookMarkTags {
			if strings.Contains(tag.Value(), pubKeyORfeedUrl) {

				copy(bookMarkTags[i:], bookMarkTags[i+1:])
				bookMarkTags[len(bookMarkTags)-1] = nostr.Tag{}
				bookMarkTags = bookMarkTags[:len(bookMarkTags)-1]

				evt := nostr.Event{
					CreatedAt: nostr.Now(),
					Kind:      KIND_BOOKMARKS,
					Content:   "{rsslay, pubkey, privkey, url, last_update}",
					Tags:      bookMarkTags,
				}

				if err := evt.Sign(s.RelayPrivkey); err != nil {
					log.Printf("[ERROR] signing event %s", err)
					return err
				}

				for _, store := range relay.StoreEvent {
					store(context.TODO(), &evt)
				}

				if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
					log.Printf("[ERROR] %s", err)
					return err
				}

				//delete related notes
				if err := deleteLocalEvents(nostr.Filter{
					Authors: []string{rsslayEntity.PublicKey},
					Kinds:   []int{nostr.KindTextNote, nostr.KindProfileMetadata}}); err != nil {
					log.Printf("[ERROR] deleting feed events: %s", err)
				}

				npub, err := nip19.EncodePublicKey(rsslayEntity.PublicKey)
				if err != nil {
					log.Printf("[ERROR] %s", err)
					return err
				}

				if err := os.Remove(fmt.Sprintf("%s/%s.png", s.QRCodePath, npub)); err != nil {
					log.Print("[ERROR] qrcode delete: ", err)
					return err
				}

				log.Printf("[DEBUG] entity %s deleted. new event ID %s saved", rsslayEntity.URL, evt.ID)
				break
			}
		}
	} else {
		log.Printf("[DEBUG] bookmark event not found")
	}

	return nil
}

// return all saved FeedURL entries
func getSavedEntries() ([]Entry, error) {

	var bookMarkTags nostr.Tags
	var rsslayEntity Entity

	var bookmarkFilter nostr.Filter = nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	bookMarkEvts, err := getLocalEvents(bookmarkFilter)
	if err != nil {
		log.Printf("[ERROR] GetLocalEvent %s", err)
		return []Entry{}, err
	}

	localEntries := make([]Entry, 0)
	if len(bookMarkEvts) > 0 {
		bookMarkTags = bookMarkEvts[0].Tags.GetAll([]string{s.RsslayTagKey})
		for _, tag := range bookMarkTags {

			if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
				log.Printf("[ERROR] %s", err)
			}

			npub, _ := nip19.EncodePublicKey(rsslayEntity.PublicKey)
			localEntries = append(localEntries, Entry{
				PubKey:  rsslayEntity.PublicKey,
				NPubKey: npub,
				Url:     rsslayEntity.URL,
			})
		}
	} else {
		log.Printf("[DEBUG] no saved feedURL entries")
	}
	return localEntries, nil
}

// find the saved Entity with the given pubkey
func getSavedEntity(pubkeyHex string) (Entity, error) {
	var bookMarkTags nostr.Tags
	var rsslayEntity Entity

	var bookmarkFilter nostr.Filter = nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	bookMarkEvts, err := getLocalEvents(bookmarkFilter)
	if err != nil {
		log.Printf("[ERROR] GetLocalEvent %s", err)
		return Entity{}, err
	}

	if len(bookMarkEvts) > 0 {
		bookMarkTags = bookMarkEvts[0].Tags.GetAll([]string{s.RsslayTagKey})
		for _, tag := range bookMarkTags {
			if strings.Contains(tag.Value(), pubkeyHex) {
				if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
					log.Printf("[ERROR] %s", err)
					return Entity{}, err
				}
				return rsslayEntity, nil
			}
		}
	}

	log.Printf("[DEBUG] feed entity not found")
	return Entity{}, nil
}

// return all feedURL Entities
func getSavedEntities() ([]Entity, error) {
	var bookMarkTags nostr.Tags
	var rsslayEntity Entity

	var bookmarkFilter nostr.Filter = nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	bookMarkEvts, err := getLocalEvents(bookmarkFilter)
	if err != nil {
		log.Printf("[ERROR] GetLocalEvent %s", err)
		return []Entity{}, err
	}

	var entities = make([]Entity, 0)
	if len(bookMarkEvts) > 0 {
		bookMarkTags = bookMarkEvts[0].Tags.GetAll([]string{s.RsslayTagKey})
		for _, tag := range bookMarkTags {
			if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
				log.Printf("[ERROR] %s", err)
			}
			entities = append(entities, rsslayEntity)
		}
	} else {
		log.Printf("[DEBUG] feed entity not found")
	}
	return entities, nil
}

// delete event from relay db
func deleteLocalEvents(filter nostr.Filter) error {
	ctx := context.TODO()

	ch, err := db.QueryEvents(ctx, filter)
	if err != nil {
		log.Printf("[ERROR] QueryEvents: %s", err)
		return err
	}

	events := make([]*nostr.Event, 0)

	for evt := range ch {
		events = append(events, evt)
	}

	if len(events) < 1 {
		log.Print("[DEBUG] no events found")
		return nil
	}

	for _, evnt := range events {
		for _, del := range relay.DeleteEvent {
			if err := del(ctx, evnt); err != nil {
				log.Printf("[ERROR] %s deleting event %s", evnt, err)
			}
		}
	}

	log.Printf("[DEBUG] %v events deleted", len(events))
	return nil
}

func deleteOldEvents() {
	if s.MaxNoteAgeDays < 1 {
		log.Printf("[INFO] MaxAgeDays disabled")
		return
	}

	maxAgeSecs := nostr.Timestamp(s.MaxNoteAgeDays * 86400)
	oldAge := nostr.Now() - maxAgeSecs
	if oldAge <= 0 {
		log.Printf("[WARN] MaxAgeDays too large")
		return
	}

	filter := nostr.Filter{
		Until: &oldAge,
		Kinds: []int{
			nostr.KindTextNote,
		},
	}

	if err := deleteLocalEvents(filter); err != nil {
		log.Printf("[ERROR] delete old notes: %s", err)
		return
	}
}

func updateFollowListEvent(followAction FollowManagment) {
	var currentOneHopNetwork []nostr.Tag

	switch followAction.Action {
	case Add:
		remoteFollows := getRemoteFollows(s.RelayPubkey)
		localFollows := getLocalFollows()
		uniqueFollows := getUniqueFollows(remoteFollows, localFollows)
		currentOneHopNetwork = append(currentOneHopNetwork, uniqueFollows...)
	case Sync:
		localFollows := getLocalFollows()
		currentOneHopNetwork = append(currentOneHopNetwork, localFollows...)
	case Delete:
		reducedFollows := deleteRemoteFollow(followAction.FollowEntity.PublicKey)
		currentOneHopNetwork = append(currentOneHopNetwork, reducedFollows...)
	}

	// evtNewSubs := nostr.Event{
	// 	PubKey:    s.RelayPubkey,
	// 	CreatedAt: nostr.Now(),
	// 	Kind:      nostr.KindFollowList,
	// 	Tags:      currentOneHopNetwork,
	// }

	// if err := evtNewSubs.Sign(s.RelayPrivkey); err != nil {
	// 	log.Printf("[ERROR] %s", err)
	// 	return
	// }

	// blastEvent(&evtNewSubs)

	log.Print("[DEBUG] 🫂 new follow list size: ", len(currentOneHopNetwork))
}

func blastEvent(ev *nostr.Event) {
	ctx := context.Background()
	for _, url := range seedRelays {
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		relay, err := pool.EnsureRelay(url)
		if err != nil {
			cancel()
			log.Printf("[ERROR] %s", err)
			continue
		}
		relay.Publish(ctx, *ev)
		cancel()
	}
	log.Print("[INFO] 🔫 blasted event ID ", ev.ID, "to ", len(seedRelays), " relays")
}
