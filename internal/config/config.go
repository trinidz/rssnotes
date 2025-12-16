package config

type C struct {
	RelayName        string `envconfig:"RELAY_NAME" default:"rssnotes"`
	RelayURL         string `envconfig:"RELAY_URL" required:"true"`
	RelayBasepath    string `envconfig:"RELAY_BASEPATH" default:"rssnotes"`
	RelayPubkey      string `envconfig:"RELAY_PUBKEY" required:"true"`
	RelayPrivkey     string `envconfig:"RELAY_PRIVKEY" required:"true"`
	RelayDescription string `envconfig:"RELAY_DESCRIPTION" default:"An rss to nostr relay."`
	RelayContact     string `envconfig:"RELAY_CONTACT" default:"example@example.com"`
	RelayIcon        string `envconfig:"RELAY_ICON" default:"https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/commafeed.png"`
	RandomSecret     string `envconfig:"RANDOM_SECRET" required:"true"`
	OwnerPubkey      string `envconfig:"OWNER_PUBKEY"`

	LogLevel       string `envconfig:"LOG_LEVEL" default:"WARN"`
	Port           string `envconfig:"PORT" default:"3334"`
	DatabasePath   string `envconfig:"DATABASE_PATH" default:"./db/rssnotes"`
	FrensdataPath  string `envconfig:"FRENSDATA_PATH" default:"./frens.json"`
	SeedRelaysPath string `envconfig:"SEED_RELAYS_PATH" default:"./seedrelays.json"`
	LogfilePath    string `envconfig:"LOGFILE_PATH" default:"./logfile.log"`
	TemplatePath   string `envconfig:"TEMPLATE_PATH" default:"./web/templates"`
	StaticPath     string `envconfig:"STATIC_PATH" default:"./web/assets"`
	QRCodePath     string `envconfig:"QRCODE_PATH" default:"./web/assets/qrcodes"`

	RsslayTagKey            string `envconfig:"RSSLAY_TAG_KEY" default:"rsslay"`
	DefaultProfilePicUrl    string `envconfig:"DEFAULT_PROFILE_PICTURE_URL" default:"/assets/static/mstile-150x150.png"`
	DeleteFailingFeeds      bool   `envconfig:"DELETE_FAILIING_FEEDS" required:"false"`
	MaxContentLength        int    `envconfig:"MAX_CONTENT_LENGTH" default:"250"`
	FeedItemsRefreshMinutes int    `envconfig:"FEED_ITEMS_REFRESH_MINUTES" default:"30"`
	FeedMetadataRefreshDays int    `envconfig:"METADATA_REFRESH_DAYS" default:"7"`
	MaxNoteAgeDays          int    `envconfig:"MAX_NOTE_AGE_DAYS" default:"0"`
	MaxBookmarkAgeHrs       int    `envconfig:"MAX_BOOKMARK_AGE_HRS" default:"1"`
	MaxAvgPostPeriodHrs     int64  `envconfig:"MAX_AVG_POST_PERIOD_HRS" default:"4"`
	MinAvgPostPeriodMins    int64  `envconfig:"MIN_AVG_POST_PERIOD_MINS" default:"10"`
	MinPostPeriodSamples    int    `envconfig:"MIN_POST_PERIOD_SAMPLES" default:"5"`
}
