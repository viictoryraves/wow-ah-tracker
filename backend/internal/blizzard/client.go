// Package blizzard is a minimal client for the Blizzard Game Data API.
package blizzard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrNotModified = errors.New("not modified")

// StatusError is returned for any non-200 response.
type StatusError struct {
	URL  string
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: status %d: %s", e.URL, e.Code, e.Body)
}

type Client struct {
	id, secret, region string
	http               *http.Client

	// Overridable for tests.
	APIBase  string // default https://<region>.api.blizzard.com
	TokenURL string // default https://oauth.battle.net/token

	mu    sync.Mutex
	token string
	exp   time.Time
}

func New(id, secret, region string) *Client {
	return &Client{
		id: id, secret: secret, region: region,
		http:     &http.Client{Timeout: 60 * time.Second},
		APIBase:  fmt.Sprintf("https://%s.api.blizzard.com", region),
		TokenURL: "https://oauth.battle.net/token",
	}
}

func (c *Client) accessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.token != "" && time.Now().Before(c.exp.Add(-time.Minute)) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.TokenURL, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.id, c.secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("oauth token: status %d: %s", resp.StatusCode, b)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	c.token = t.AccessToken
	c.exp = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	return c.token, nil
}

// Result carries the decoded body plus the Last-Modified header, if any.
type Result struct {
	LastModified time.Time
}

// GetJSON GETs path (or a full URL) in the given namespace and decodes the JSON body into out.
// If ifModifiedSince is non-zero a conditional request is made and ErrNotModified is returned on 304.
func (c *Client) GetJSON(ctx context.Context, path, namespace string, ifModifiedSince time.Time, out interface{}) (Result, error) {
	var res Result
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := c.accessToken(ctx, attempt > 0)
		if err != nil {
			return res, err
		}
		u := path
		if !strings.HasPrefix(u, "http") {
			u = c.APIBase + path
		}
		if i := strings.Index(u, "?"); i >= 0 {
			u = u[:i]
		}
		q := url.Values{"namespace": {namespace}, "locale": {"en_GB"}}
		req, err := http.NewRequestWithContext(ctx, "GET", u+"?"+q.Encode(), nil)
		if err != nil {
			return res, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if !ifModifiedSince.IsZero() {
			req.Header.Set("If-Modified-Since", ifModifiedSince.UTC().Format(http.TimeFormat))
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return res, err
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			resp.Body.Close()
			continue
		case resp.StatusCode == http.StatusNotModified:
			resp.Body.Close()
			return res, ErrNotModified
		case resp.StatusCode != http.StatusOK:
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			resp.Body.Close()
			return res, &StatusError{URL: u, Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
		}
		if lm := resp.Header.Get("Last-Modified"); lm != "" {
			if t, err := http.ParseTime(lm); err == nil {
				res.LastModified = t
			}
		}
		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return res, err
	}
	return res, errors.New("unauthorized after token refresh")
}

// Auction is one listing; Buyout is the total price in copper.
type Auction struct {
	ID       int64 // the listing's own id: it identifies the same listing across snapshots
	ItemID   int
	Buyout   int64 // total price in copper
	Quantity int
	TimeLeft string // SHORT (<30m), MEDIUM (<2h), LONG (<12h) or VERY_LONG
}

// Ruleset is a WoW Forever ruleset (PvP, Normal, Roleplay, Roleplay PvP). Forever
// has no realms: auction houses belong to a ruleset and a faction.
type Ruleset struct {
	ID   int
	Name string
}

// Rulesets lists the Forever rulesets (index plus each ruleset's name).
func (c *Client) Rulesets(ctx context.Context, namespace string) ([]Ruleset, error) {
	var idx struct {
		Rulesets []struct {
			ID int `json:"id"`
		} `json:"rulesets"`
	}
	if _, err := c.GetJSON(ctx, "/data/wow/ruleset/index", namespace, time.Time{}, &idx); err != nil {
		return nil, err
	}
	out := make([]Ruleset, 0, len(idx.Rulesets))
	for _, r := range idx.Rulesets {
		var d struct {
			Name string `json:"name"`
		}
		if _, err := c.GetJSON(ctx, fmt.Sprintf("/data/wow/ruleset/%d", r.ID), namespace, time.Time{}, &d); err != nil {
			return nil, err
		}
		out = append(out, Ruleset{ID: r.ID, Name: d.Name})
	}
	return out, nil
}

// RulesetAuctions fetches one Forever house. Each house has two documents:
// regular listings (buyout is a total price) and commodities (stackables, priced
// per unit). Commodities are converted to a total buyout so both read alike.
func (c *Client) RulesetAuctions(ctx context.Context, namespace string, rulesetID, houseID int, commodities bool, since time.Time) ([]Auction, time.Time, error) {
	var doc struct {
		Auctions []struct {
			ID   int64 `json:"id"`
			Item struct {
				ID int `json:"id"`
			} `json:"item"`
			Buyout    int64  `json:"buyout"`
			UnitPrice int64  `json:"unit_price"`
			Quantity  int    `json:"quantity"`
			TimeLeft  string `json:"time_left"`
		} `json:"auctions"`
	}
	path := fmt.Sprintf("/data/wow/ruleset/%d/auctions/%d", rulesetID, houseID)
	if commodities {
		path += "/commodities"
	}
	res, err := c.GetJSON(ctx, path, namespace, since, &doc)
	if err != nil {
		return nil, time.Time{}, err
	}
	out := make([]Auction, 0, len(doc.Auctions))
	for _, a := range doc.Auctions {
		buyout := a.Buyout
		if commodities {
			buyout = a.UnitPrice * int64(a.Quantity)
		}
		out = append(out, Auction{ID: a.ID, ItemID: a.Item.ID, Buyout: buyout, Quantity: a.Quantity, TimeLeft: a.TimeLeft})
	}
	return out, res.LastModified, nil
}

// ItemInfo is what we show for an item.
type ItemInfo struct {
	Name string
	Icon string // icon URL; empty when the item has none
	// VendorSell is what a vendor pays for one, in copper (0 when they do not buy it).
	// It is the floor for the price worth asking at the auction house.
	VendorSell int64
}

// ItemInfo fetches an item's name and icon from the static namespace. A
// missing icon (404) is not an error; any other icon failure is, so the caller
// retries later instead of recording "no icon".
func (c *Client) ItemInfo(ctx context.Context, staticNamespace string, id int) (ItemInfo, error) {
	var item struct {
		Name      string `json:"name"`
		SellPrice int64  `json:"sell_price"`
	}
	if _, err := c.GetJSON(ctx, fmt.Sprintf("/data/wow/item/%d", id), staticNamespace, time.Time{}, &item); err != nil {
		return ItemInfo{}, err
	}
	info := ItemInfo{Name: item.Name, VendorSell: item.SellPrice}

	var media struct {
		Assets []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"assets"`
	}
	_, err := c.GetJSON(ctx, fmt.Sprintf("/data/wow/media/item/%d", id), staticNamespace, time.Time{}, &media)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return info, nil
	}
	if err != nil {
		return ItemInfo{}, err
	}
	for _, a := range media.Assets {
		if a.Key == "icon" {
			info.Icon = a.Value
			break
		}
	}
	return info, nil
}
