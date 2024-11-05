package main

import (
	"fmt"

	"github.com/mmcdole/gofeed"
	"github.com/mmcdole/gofeed/rss"
)

type CustomTranslator struct {
	defaultRSSTranslator *gofeed.DefaultRSSTranslator
}

func NewCustomTranslator() *CustomTranslator {
	t := &CustomTranslator{}

	t.defaultRSSTranslator = &gofeed.DefaultRSSTranslator{}
	return t
}

func (ct *CustomTranslator) Translate(feed interface{}) (*gofeed.Feed, error) {
	rssFeed, found := feed.(*rss.Feed)
	if !found {
		return nil, fmt.Errorf("feed did not match expected type of *rss.Feed")
	}

	f, err := ct.defaultRSSTranslator.Translate(rssFeed)
	if err != nil {
		return nil, err
	}

	for i, item := range rssFeed.Items {
		if item.Comments != "" {
			if f.Items[i].Custom == nil {
				f.Items[i].Custom = map[string]string{}
			}
			f.Items[i].Custom["comments"] = item.Comments
		}
	}

	return f, nil
}
