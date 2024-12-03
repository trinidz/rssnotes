package htmlutil

import (
	"net/url"
)

func AbsoluteUrl(href, base string) string {
	baseUrl, err := url.Parse(base)
	if err != nil {
		return ""
	}
	hrefUrl, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return baseUrl.ResolveReference(hrefUrl).String()
}
