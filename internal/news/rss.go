package news

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The longest headline and summary a story keeps, in characters. A summary
// is the publisher's blurb, never the article: a feed that puts the article
// in its description (some do) is cut here, and the store refuses longer
// (PRD §3; AGENTS.md, Text from outside is data).
const (
	MaxTitle   = 300
	MaxSummary = 500
	maxGUID    = 512
	maxLink    = 2048
)

// Item is one story as a feed gives it, already made safe: plain text, an
// https link or none, a time or the zero time for unknown.
type Item struct {
	GUID      string
	URL       string // "" when the feed's link is not one we store
	Title     string
	Summary   string
	Published time.Time // zero when the feed gives none we can read
}

// rssItem has a field for each part of an item Eunomia keeps, and no other:
// content:encoded, which carries the article, is never read into memory.
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

type rssDoc struct {
	Items []rssItem `xml:"channel>item"`
}

// Parse reads an RSS 2.0 document into items for feed f: headlines and
// summaries as plain text, links that are https (or http on the feed's own
// site, upgraded), one item per guid. An item with no headline, or neither a
// guid nor a link, is skipped. encoding/xml resolves no external entities.
func Parse(body []byte, f Feed) ([]Item, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = func(charset string, r io.Reader) (io.Reader, error) {
		switch strings.ToLower(charset) {
		case "", "utf-8", "us-ascii":
			return r, nil
		}
		return nil, fmt.Errorf("feed %s: unsupported charset %q", f.ID, charset)
	}
	var doc rssDoc
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse feed %s: %w", f.ID, err)
	}
	items := make([]Item, 0, len(doc.Items))
	seen := map[string]bool{}
	for _, r := range doc.Items {
		title := cut(plainText(r.Title), MaxTitle)
		guid := strings.TrimSpace(r.GUID)
		if guid == "" {
			guid = strings.TrimSpace(r.Link)
		}
		if title == "" || guid == "" || len(guid) > maxGUID || seen[guid] {
			continue
		}
		seen[guid] = true
		items = append(items, Item{
			GUID:      guid,
			URL:       safeLink(r.Link, f.Site),
			Title:     title,
			Summary:   cut(plainText(r.Description), MaxSummary),
			Published: parsePublished(r.PubDate),
		})
	}
	return items, nil
}

var (
	reScript = regexp.MustCompile(`(?is)<script\b.*?</script\s*>`)
	reStyle  = regexp.MustCompile(`(?is)<style\b.*?</style\s*>`)
	reTag    = regexp.MustCompile(`(?s)<[^>]*>`)
)

// plainText turns a feed's text, which may hold HTML, into plain text: a
// script's or style's content dropped, every tag dropped, entities decoded,
// control characters removed and whitespace collapsed. The result is shown
// as text, never as HTML, so a tag this misses is shown, not run.
func plainText(s string) string {
	s = reScript.ReplaceAllString(s, " ")
	s = reStyle.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// cut shortens s to at most n characters, at a word, ending in "…".
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)[:n-1]
	text := string(runes)
	if i := strings.LastIndexFunc(text, unicode.IsSpace); i > len(text)/2 {
		text = text[:i]
	}
	return strings.TrimRightFunc(text, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }) + "…"
}

// safeLink returns an https link, upgrading http on the feed's own site
// (which serves https), and "" for anything else (go-conventions, Security:
// URLs from outside are stored only if they parse and are https).
func safeLink(raw, site string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return ""
	}
	switch {
	case u.Scheme == schemeHTTPS:
	case u.Scheme == "http" && strings.EqualFold(u.Hostname(), site):
		u.Scheme = schemeHTTPS
	default:
		return ""
	}
	out := u.String()
	if len(out) > maxLink {
		return ""
	}
	return out
}

// zones are the RFC 822 zone names feeds use; Go's time.Parse would read an
// unknown name as UTC, which would be wrong for the US ones.
var zones = map[string]string{
	"GMT": "+0000", "UT": "+0000", "UTC": "+0000", "Z": "+0000",
	"EST": "-0500", "EDT": "-0400", "CST": "-0600", "CDT": "-0500",
	"MST": "-0700", "MDT": "-0600", "PST": "-0800", "PDT": "-0700",
}

var dateLayouts = []string{
	"Mon, _2 Jan 2006 15:04:05 -0700",
	"_2 Jan 2006 15:04:05 -0700",
	"Mon, _2 Jan 2006 15:04 -0700",
	time.RFC3339,
}

// parsePublished reads a feed's pubDate as UTC, or returns the zero time
// (unknown) for one it cannot read: never a guess.
func parsePublished(raw string) time.Time {
	s := strings.TrimSpace(raw)
	if i := strings.LastIndexByte(s, ' '); i > 0 {
		if off, ok := zones[strings.ToUpper(s[i+1:])]; ok {
			s = s[:i+1] + off
		}
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
