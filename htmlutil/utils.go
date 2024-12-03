package htmlutil

import (
	"strings"

	"golang.org/x/net/html"
)

func Attr(node *html.Node, key string) string {
	for _, a := range node.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}
