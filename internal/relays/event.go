package relays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"rssnotes/internal/models"
	"rssnotes/metrics"
	"sort"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

const KIND_BOOKMARKS int = 10003 //NIP-51
var BlastNostrEventCh = make(chan nostr.Event, 50)

// get nostr events from local relay.
// events are returned in an array sorted from oldest [len-1] to most current [0] event
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

// get the most recent bookmark event
func getBookMarkEvent() (*nostr.Event, error) {
	filter := nostr.Filter{
		Kinds:   []int{KIND_BOOKMARKS},
		Authors: []string{s.RelayPubkey},
	}

	events, err := getLocalEvents(filter)
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return nil, err
	}

	if len(events) == 0 {
		return nil, nil
	}

	return events[0], nil
}

// get kind-0 metadata event of a pubkey
func GetLocalMetadataEvent(pubkey string) (nostr.Event, error) {

	metaDataFilter := nostr.Filter{
		Kinds:   []int{nostr.KindProfileMetadata},
		Authors: []string{pubkey},
	}

	metaData, err := getLocalEvents(metaDataFilter)
	if err != nil {
		log.Print("[ERROR]", err)
		return nostr.Event{}, err
	}

	if len(metaData) == 0 {
		log.Printf("[DEBUG] no profile data found for pubkey %s", pubkey)
		return nostr.Event{}, nil
	}

	return *metaData[0], nil
}

// TRUE if feed exists in bookmark event
func FeedExists(pubkeyHex, privKeyHex, feedUrl string) (bool, error) {

	if feedUrl == "" {
		log.Printf("[ERROR] feedURL is empty")
		return false, fmt.Errorf("feedURL is empty")
	}

	bookmarkEvent, err := getBookMarkEvent()
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return false, err
	}

	if bookmarkEvent == nil {
		log.Printf("[DEBUG] no bookmark found")
		return false, nil
	}

	bookMarkTags := bookmarkEvent.Tags.GetAll([]string{s.RsslayTagKey})
	for _, tag := range bookMarkTags {
		if strings.Contains(tag.Value(), pubkeyHex) || strings.Contains(tag.Value(), privKeyHex) || strings.Contains(tag.Value(), feedUrl) {
			log.Printf("[DEBUG] feedUrl %s already exists", feedUrl)
			return true, nil
		}
	}

	log.Printf("[DEBUG] feed %s does not exist", feedUrl)
	return false, nil
}

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

		switch evnt.Kind {
		case nostr.KindTextNote:
			metrics.KindTextNoteDeleted.Inc()
		case nostr.KindProfileMetadata:
			metrics.KindProfileMetadatasDeleted.Inc()
		case KIND_BOOKMARKS:
			metrics.KindBookmarkNotesDeleted.Inc()
		}

		for _, del := range rly.DeleteEvent {
			if err := del(ctx, evnt); err != nil {
				log.Printf("[ERROR] %s deleting event %s", evnt, err)
			}
		}
	}

	log.Printf("[DEBUG] %v events deleted", len(events))
	return nil
}

func DeleteOldKindTextNoteEvents() {
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

func deleteOldKBookmarkEvents() {
	if s.MaxBookmarkAgeHrs < 1 {
		log.Printf("[INFO] MaxBookmarkAgeHrs disabled")
		return
	}

	maxAgeSecs := nostr.Timestamp(s.MaxBookmarkAgeHrs * 60 * 60)
	oldAge := nostr.Now() - maxAgeSecs
	if oldAge <= 0 {
		log.Printf("[WARN] MaxBookmarkAgeHrs too large")
		return
	}

	filter := nostr.Filter{
		Until: &oldAge,
		Limit: 10,
		Kinds: []int{
			KIND_BOOKMARKS,
		},
	}

	if err := deleteLocalEvents(filter); err != nil {
		log.Printf("[ERROR] delete old notes: %s", err)
		return
	}
}

func UpdateFollowListEvent(followAction models.FollowManagment) {
	var currentOneHopNetwork []nostr.Tag

	switch followAction.Action {
	case models.Add:
		remoteFollows := getRemoteFollows(s.RelayPubkey)
		localFollows := getLocalFollows()
		uniqueFollows := getUniqueFollows(remoteFollows, localFollows)
		currentOneHopNetwork = append(currentOneHopNetwork, uniqueFollows...)
	case models.Sync:
		localFollows := getLocalFollows()
		currentOneHopNetwork = append(currentOneHopNetwork, localFollows...)
	case models.Delete:
		//reducedFollows := deleteRemoteFollow(followAction.FollowEntity.PublicKey)
		reducedFollows := deleteLocalFollow(followAction.FollowEntity.PubKey)
		if reducedFollows != nil {
			currentOneHopNetwork = append(currentOneHopNetwork, reducedFollows...)
		} else {
			return
		}
	}

	evtNewSubs := nostr.Event{
		PubKey:    s.RelayPubkey,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindFollowList,
		Tags:      currentOneHopNetwork,
	}

	if err := evtNewSubs.Sign(s.RelayPrivkey); err != nil {
		log.Printf("[ERROR] %s", err)
		return
	}

	rly.BroadcastEvent(&evtNewSubs)

	for _, store := range rly.StoreEvent {
		if err := store(context.TODO(), &evtNewSubs); err != nil {
			log.Printf("[ERROR] %s", err)
		}
	}

	// blastEvent(&evtNewSubs)

	log.Print("[DEBUG] 🫂 new follow list size: ", len(currentOneHopNetwork))
}

func BlastWorker(queue <-chan nostr.Event) {
	ctx := context.Background()
	for ev := range queue {
		successRelays := 0
		for _, url := range seedRelays {
			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			relay, err := pool.EnsureRelay(url)
			if err != nil {
				cancel()
				log.Printf("[ERROR] %s", err)
				continue
			}

			if err := relay.Publish(ctx, ev); err == nil {
				successRelays++
				log.Printf("[DEBUG] event %s blasted to relay %s ", ev.ID, url)
			} else {
				log.Printf("[ERROR] %s: %s", url, err)
			}
			cancel()
		}

		if successRelays > 0 {
			metrics.NotesBlasted.Inc()
			log.Printf("[INFO] 🔫 blasted event ID %s to %d relays", ev.ID, successRelays)
		}
	}
}

// adds entities to bookmark event
func AddEntity(entitiesToAdd []models.Entity) error {
	if len(entitiesToAdd) == 0 {
		return nil
	}

	bookmarkEvent, err := getBookMarkEvent()
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return err
	}

	var bookMarkTags nostr.Tags

	if bookmarkEvent != nil {
		bookMarkTags = bookmarkEvent.Tags.GetAll([]string{s.RsslayTagKey})
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
		Content:   "rsslay -> rssnotes",
		Tags:      bookMarkTags,
	}

	if err := evt.Sign(s.RelayPrivkey); err != nil {
		log.Printf("[ERROR] signing event %s", err)
		return err
	}

	for _, store := range rly.StoreEvent {
		store(context.TODO(), &evt)
	}
	metrics.KindBookmarkNotesCreated.Inc()

	log.Printf("[DEBUG] bookmark event %s stored", evt.ID)
	return nil
}

// change properties of an entity
func UpdateEntity(entityPubkey string, opts ...models.Option) error {

	if !nostr.IsValidPublicKey(entityPubkey) {
		return fmt.Errorf("[ERROR] bad pubkey: %s", entityPubkey)
	}

	bookmarkEvent, err := getBookMarkEvent()
	if err != nil {
		log.Printf("[ERROR] %v", err)
		return err
	}

	if bookmarkEvent == nil {
		log.Printf("[ERROR] bookmark event is nil")
		return errors.New("bookmark event is nil")
	}

	var updatedEntity models.Entity

	bookMarkTags := bookmarkEvent.Tags.GetAll([]string{s.RsslayTagKey})
	for i, tag := range bookMarkTags {
		if strings.Contains(tag.Value(), entityPubkey) {
			if err := json.Unmarshal([]byte(tag.Value()), &updatedEntity); err != nil {
				log.Printf("[ERROR] %s", err)
			}

			copy(bookMarkTags[i:], bookMarkTags[i+1:])
			bookMarkTags[len(bookMarkTags)-1] = nostr.Tag{}
			bookMarkTags = bookMarkTags[:len(bookMarkTags)-1]

			_, err := models.UpdateEntity(&updatedEntity, opts...)
			if err != nil {
				log.Printf("[ERROR] %v\n", err)
				return err
			}

			jsonentArr, err := json.Marshal(updatedEntity)
			if err != nil {
				log.Printf("[ERROR] %s", err)
				return err
			}

			bookMarkTags = append(bookMarkTags, nostr.Tag{s.RsslayTagKey, string(jsonentArr)})

			evt := nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      KIND_BOOKMARKS,
				Content:   "",
				Tags:      bookMarkTags,
			}

			if err := evt.Sign(s.RelayPrivkey); err != nil {
				log.Printf("[ERROR] signing event %s", err)
				return err
			}

			for _, store := range rly.StoreEvent {
				store(context.TODO(), &evt)
			}

			metrics.KindBookmarkNotesCreated.Inc()

			log.Printf("[DEBUG] entity %s updated in event ID %s. avg post time %d", updatedEntity.FeedURL, evt.ID, updatedEntity.AvgPostTime)
			break
		}
	}

	return nil
}

func DeleteEntity(entityPubkey string) error {

	if !nostr.IsValidPublicKey(entityPubkey) {
		return fmt.Errorf("[ERROR] bad pubkey: %s", entityPubkey)
	}

	bookmarkEvent, err := getBookMarkEvent()
	if err != nil {
		log.Printf("[ERROR] %v", err)
		return err
	}

	if bookmarkEvent == nil {
		log.Printf("[ERROR] bookmark event is nil")
		return errors.New("bookmark event is nil")
	}

	bookmarkTags := bookmarkEvent.Tags.GetAll([]string{s.RsslayTagKey})
	for i, tag := range bookmarkTags {
		if strings.Contains(tag.Value(), entityPubkey) {

			copy(bookmarkTags[i:], bookmarkTags[i+1:])
			bookmarkTags[len(bookmarkTags)-1] = nostr.Tag{}
			bookmarkTags = bookmarkTags[:len(bookmarkTags)-1]

			evt := nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      KIND_BOOKMARKS,
				Content:   "{rsslay, pubkey, privkey, url, last_update}",
				Tags:      bookmarkTags,
			}

			if err := evt.Sign(s.RelayPrivkey); err != nil {
				log.Printf("[ERROR] signing event %s", err)
				return err
			}

			for _, store := range rly.StoreEvent {
				store(context.TODO(), &evt)
			}
			metrics.KindBookmarkNotesCreated.Inc()

			var rsslayEntity models.Entity
			if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
				log.Printf("[ERROR] %s", err)
				return err
			}

			//delete related notes
			if err := deleteLocalEvents(nostr.Filter{
				Authors: []string{rsslayEntity.PubKey},
				Kinds:   []int{nostr.KindTextNote, nostr.KindProfileMetadata}}); err != nil {
				log.Printf("[ERROR] deleting feed events: %s", err)
			}

			npub, err := nip19.EncodePublicKey(rsslayEntity.PubKey)
			if err != nil {
				log.Printf("[ERROR] %s", err)
				return err
			}

			if err := os.Remove(fmt.Sprintf("%s/%s.png", s.QRCodePath, npub)); err != nil {
				log.Print("[ERROR] qrcode delete: ", err)
				return err
			}

			log.Printf("[DEBUG] entity %s deleted. new event ID %s saved", rsslayEntity.FeedURL, evt.ID)
			break
		}
	}

	return nil
}

func GetEntries() ([]models.GUIEntry, error) {

	var localEntries []models.GUIEntry

	ents, err := GetEntities()
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return []models.GUIEntry{}, err
	}

	for _, ent := range ents {
		localEntries = append(localEntries, models.GUIEntry{BookmarkEntity: ent})
	}

	return localEntries, nil
}

// get entity by its pubkey
func GetEntity(pubkeyHex string) (models.Entity, error) {

	if !nostr.IsValidPublicKey(pubkeyHex) {
		return models.Entity{}, fmt.Errorf("[ERROR] bad pubkey")
	}

	bookmarkEvent, err := getBookMarkEvent()
	if err != nil {
		log.Printf("[ERROR] %v", err)
		return models.Entity{}, err
	}

	if bookmarkEvent == nil {
		log.Printf("[ERROR] bookmark event is nil")
		return models.Entity{}, errors.New("bookmark event is nil")
	}

	var rsslayEntity models.Entity

	bookMarkTags := bookmarkEvent.Tags.GetAll([]string{s.RsslayTagKey})
	for _, tag := range bookMarkTags {
		if strings.Contains(tag.Value(), pubkeyHex) {
			if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
				log.Printf("[ERROR] %s", err)
				return models.Entity{}, err
			}
			return rsslayEntity, nil
		}
	}

	log.Printf("[DEBUG] entity for pubkey %s not found", pubkeyHex)
	return models.Entity{}, nil
}

// get all entities
func GetEntities() ([]models.Entity, error) {

	bookmarkEvent, err := getBookMarkEvent()
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return nil, err
	}

	if bookmarkEvent == nil {
		log.Printf("[DEBUG] bookmark event is nil")
		return nil, nil
	}

	var entities = make([]models.Entity, 0)
	var rsslayEntity models.Entity

	bookMarkTags := bookmarkEvent.Tags.GetAll([]string{s.RsslayTagKey})
	for _, tag := range bookMarkTags {
		if err := json.Unmarshal([]byte(tag.Value()), &rsslayEntity); err != nil {
			log.Printf("[ERROR] %s", err)
		}
		entities = append(entities, rsslayEntity)
	}

	return entities, nil
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

	savedEnts, err := GetEntities()
	if err != nil {
		log.Printf("[ERROR] Can not get local follows %s", err)
		return nil
	}

	for _, savedEnt := range savedEnts {
		localFollows = append(localFollows, nostr.Tag{"p", savedEnt.PubKey})
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

// delete a follow from local kind 3 event
func deleteLocalFollow(pubkeyHex string) nostr.Tags {
	localFollows := getLocalFollows()

	for i, localFollow := range localFollows {
		if localFollow.Value() == pubkeyHex {
			copy(localFollows[i:], localFollows[i+1:])
			localFollows[len(localFollows)-1] = nostr.Tag{}
			localFollows = localFollows[:len(localFollows)-1]

			return localFollows
		}
	}

	return nil
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
