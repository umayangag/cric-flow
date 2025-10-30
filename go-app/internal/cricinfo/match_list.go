package cricinfo

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ExtractMatchList fetches a Cricinfo team results URL and parses the innings list.
// It mirrors the Python src/scrapers/match_list.py but aims to be a bit more resilient.
func ExtractMatchList(url string) ([]MatchListItem, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []MatchListItem

	// Attempt 1: follow the original logic that used tbody[2]
	// We try to find all tbodys and fall back if not enough bodies exist
	bodies := doc.Find("tbody")
	if bodies.Length() >= 3 {
		bodies.Eq(2).Find("tr").Each(func(_ int, tr *goquery.Selection) {
			cols := tr.Find("td")
			if cols.Length() < 11 {
				return
			}
			scoreText := strings.TrimSpace(cols.Eq(0).Text())
			wickets := 10
			scoreParts := strings.Split(scoreText, "/")
			if len(scoreParts) > 0 && strings.EqualFold(scoreParts[0], "DNB") {
				wickets = 0
			}
			if len(scoreParts) == 2 {
				if v, err := strconv.Atoi(strings.TrimSpace(scoreParts[1])); err == nil {
					wickets = v
				}
			}
			// Match id from last link
			link := cols.Eq(10).Find("a").First()
			matchHref, _ := link.Attr("href")
			matchID := ""
			if matchHref != "" {
				parts := strings.Split(matchHref, "/")
				if len(parts) >= 5 {
					matchID = strings.TrimSuffix(parts[4], ".html")
				}
			}

			item := MatchListItem{
				Score:      firstOrEmpty(scoreParts),
				Wickets:    wickets,
				Overs:      strings.TrimSpace(cols.Eq(1).Text()),
				RPO:        strings.TrimSpace(cols.Eq(2).Text()),
				Target:     strings.TrimSpace(cols.Eq(3).Text()),
				Inning:     strings.TrimSpace(cols.Eq(4).Text()),
				Result:     strings.TrimSpace(cols.Eq(5).Text()),
				Opposition: strings.TrimSpace(cols.Eq(7).Find("a").First().Text()),
				Ground:     strings.TrimSpace(cols.Eq(8).Text()),
				DateText:   strings.TrimSpace(cols.Eq(9).Text()),
				MatchID:    matchID,
				URLText:    strings.TrimSpace(cols.Eq(10).Find("a").First().Text()),
			}
			items = append(items, item)
		})
	}

	return items, nil
}

func firstOrEmpty(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
