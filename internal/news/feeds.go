// Package news reads the configured RSS feeds into stories: headline, a
// short plain-text summary, publication time and a link, never an article's
// text (AGENTS.md, Text from outside is data; PRD §3). The feeds are listed
// in config/sources.yaml.
package news

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"slices"
	"time"

	"go.yaml.in/yaml/v3"
)

// Feed is one configured news feed (config/sources.yaml).
type Feed struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Site    string `yaml:"site"`
	Label   string `yaml:"label"`
	Source  string `yaml:"source"`
	Checked string `yaml:"checked"`
}

var (
	reFeedID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
	reHost   = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,252}$`)
)

// LoadFeeds reads and validates sources.yaml from fsys, reporting every
// problem at once, each naming its feed. A feed id may not be one of
// reserved, the other sources' names, which share its label and its row.
func LoadFeeds(fsys fs.FS, reserved ...string) ([]Feed, error) {
	b, err := fs.ReadFile(fsys, "sources.yaml")
	if err != nil {
		return nil, fmt.Errorf("read sources.yaml: %w", err)
	}
	var doc struct {
		Feeds []Feed `yaml:"feeds"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse sources.yaml: %w", err)
	}
	if len(doc.Feeds) == 0 {
		return nil, errors.New("sources.yaml: no feeds")
	}
	var errs []error
	seen := map[string]bool{}
	for i, f := range doc.Feeds {
		where := fmt.Sprintf("sources.yaml feed %d (%s)", i+1, f.ID)
		if !reFeedID.MatchString(f.ID) {
			errs = append(errs, fmt.Errorf("%s: id must be a short lower-case slug", where))
		}
		if seen[f.ID] {
			errs = append(errs, fmt.Errorf("%s: id is listed twice", where))
		}
		if slices.Contains(reserved, f.ID) {
			// The id is the fetch metric's source label and the key of the
			// feed's row in sources: another source's name would share both.
			errs = append(errs, fmt.Errorf("%s: id is another source's name", where))
		}
		seen[f.ID] = true
		if f.Name == "" || len(f.Name) > 60 {
			errs = append(errs, fmt.Errorf("%s: name must be 1 to 60 characters", where))
		}
		if f.Label == "" || len(f.Label) > 60 {
			errs = append(errs, fmt.Errorf("%s: label must be 1 to 60 characters", where))
		}
		if !isHTTPS(f.URL) {
			errs = append(errs, fmt.Errorf("%s: url must be an https URL", where))
		}
		if !isHTTPS(f.Source) {
			errs = append(errs, fmt.Errorf("%s: source must be an https URL", where))
		}
		if !reHost.MatchString(f.Site) {
			errs = append(errs, fmt.Errorf("%s: site must be a host name", where))
		}
		if _, err := time.Parse(time.DateOnly, f.Checked); err != nil {
			errs = append(errs, fmt.Errorf("%s: checked must be a date (YYYY-MM-DD)", where))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return doc.Feeds, nil
}

const schemeHTTPS = "https"

func isHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == schemeHTTPS && u.Host != ""
}
