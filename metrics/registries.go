package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	IndexRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_index_ops_total",
		Help: "The total number of processed index requests",
	})

	ImportRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_import_ops_total",
		Help: "The total number of processed import requests",
	})

	DeleteRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_delete_ops_total",
		Help: "The total number of processed delete requests",
	})

	SearchRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_search_ops_total",
		Help: "The total number of processed search requests",
	})
	CreateRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_create_ops_total",
		Help: "The total number of processed create feed requests",
	})
	CreateRequestsAPI = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_create_api_ops_total",
		Help: "The total number of processed create feed requests via API",
	})
	WellKnownRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_wellknown_ops_total",
		Help: "The total number of processed well-known requests",
	})
	RelayInfoRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_relay_info_ops_total",
		Help: "The total number of processed relay info requests",
	})
	QueryEventsRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_query_events_ops_total",
		Help: "The total number of processed query events requests",
	})
	InvalidEventsRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_invalid_events_ops_total",
		Help: "The total number of processed invalid events requests",
	})

	KindProfileMetadataCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_kind_zero_notes_created_total",
		Help: "The total number of kind zero notes created",
	})
	KindProfileMetadatasDeleted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_kind_zero_notes_deleted_total",
		Help: "The total number of kind zero notes deleted",
	})
	KindTextNoteCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_kind_one_notes_created_total",
		Help: "The total number of kind one notes created",
	})
	KindTextNoteDeleted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_kind_one_notes_deleted_total",
		Help: "The total number of kind one notes deleted",
	})
	KindBookmarkNotesCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_kind_bookmark_notes_created_total",
		Help: "The total number of kind bookmark notes created",
	})
	KindBookmarkNotesDeleted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_kind_bookmark_notes_deleted_total",
		Help: "The total number of kind bookmark notes deleted",
	})

	NotesBlasted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_notes_blasted_total",
		Help: "The total number of notes blasted",
	})
	CacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_cache_hits_ops_total",
		Help: "The total number of cache hits",
	})
	CacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rsslay_processed_cache_miss_ops_total",
		Help: "The total number of cache misses",
	})
	AppErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rsslay_errors_total",
		Help: "Number of errors for the app.",
	}, []string{"type"})
	ReplayRoutineQueueLength = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rsslay_replay_routines_queue_length",
		Help: "Current number of subroutines to replay events to other relays",
	})
	ReplayEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rsslay_replay_events_total",
		Help: "Number of correct replayed events by relay.",
	}, []string{"relay"})
	ReplayErrorEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rsslay_replay_events_error_total",
		Help: "Number of error replayed events by relay.",
	}, []string{"relay"})
)
