package shortener

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type Shortener struct {
	apiKey  string
	site    string
	enabled bool
	client  *http.Client

	mu    sync.Mutex
	cache *lruCache
	sf    singleflight.Group
}

type lruCache struct {
	cap   int
	items map[string]*list.Element
	order *list.List
}

type lruEntry struct {
	key   string
	value string
}

func newLRUCache(cap int) *lruCache {
	return &lruCache{cap: cap, items: make(map[string]*list.Element, cap), order: list.New()}
}

func (l *lruCache) get(key string) (string, bool) {
	if l == nil {
		return "", false
	}
	el, ok := l.items[key]
	if !ok {
		return "", false
	}
	l.order.MoveToFront(el)
	return el.Value.(*lruEntry).value, true
}

func (l *lruCache) put(key, value string) {
	if l == nil {
		return
	}
	if el, ok := l.items[key]; ok {
		el.Value.(*lruEntry).value = value
		l.order.MoveToFront(el)
		return
	}
	el := l.order.PushFront(&lruEntry{key: key, value: value})
	l.items[key] = el
	if l.order.Len() > l.cap {
		oldest := l.order.Back()
		if oldest != nil {
			l.order.Remove(oldest)
			delete(l.items, oldest.Value.(*lruEntry).key)
		}
	}
}

type shortenerResponse struct {
	ShortURL     string `json:"shorturl"`
	URL          string `json:"url"`
	Short        string `json:"short"`
	ShortenedURL string `json:"shortenedUrl"`
	Data         struct {
		URL string `json:"url"`
	} `json:"data"`
}

func NewURLShortener(site, apiKey string, enabled bool) *Shortener {
	cleanSite := strings.TrimRight(strings.TrimSpace(site), "/")
	cleanKey := strings.TrimSpace(apiKey)
	isEnabled := enabled && cleanSite != "" && cleanKey != ""

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: transport,
	}

	return &Shortener{
		apiKey:  cleanKey,
		site:    cleanSite,
		enabled: isEnabled,
		client:  client,
		cache:   newLRUCache(10000),
	}
}

func (s *Shortener) Enabled() bool {
	return s != nil && s.enabled
}

func (s *Shortener) Shorten(ctx context.Context, longURL string) string {
	if s == nil || !s.enabled || longURL == "" {
		return longURL
	}

	s.mu.Lock()
	if cached, ok := s.cache.get(longURL); ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	v, _, _ := s.sf.Do(longURL, func() (any, error) {
		return s.callAPI(context.Background(), longURL), nil
	})
	short, _ := v.(string)
	if short == "" || short == longURL {
		return longURL
	}

	s.mu.Lock()
	s.cache.put(longURL, short)
	s.mu.Unlock()

	return short
}

func (s *Shortener) callAPI(ctx context.Context, longURL string) string {
	siteLower := strings.ToLower(s.site)
	var reqURL string

	switch {
	case strings.Contains(siteLower, "ouo.io"):
		reqURL = fmt.Sprintf("http://ouo.io/api/%s?s=%s", s.apiKey, url.QueryEscape(longURL))
	case strings.Contains(siteLower, "cutt.ly"):
		reqURL = fmt.Sprintf("http://cutt.ly/api/api.php?key=%s&short=%s", s.apiKey, url.QueryEscape(longURL))
	default:
		proto := "https"
		if !strings.HasPrefix(s.site, "http://") && !strings.HasPrefix(s.site, "https://") {
			reqURL = fmt.Sprintf("%s://%s/api?api=%s&url=%s", proto, s.site, s.apiKey, url.QueryEscape(longURL))
		} else {
			reqURL = fmt.Sprintf("%s/api?api=%s&url=%s", s.site, s.apiKey, url.QueryEscape(longURL))
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return longURL
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return longURL
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return longURL
	}

	bodyStr := strings.TrimSpace(string(body))
	if strings.HasPrefix(bodyStr, "{") {
		var sr shortenerResponse
		if err := json.Unmarshal([]byte(bodyStr), &sr); err == nil {
			for _, val := range []string{sr.ShortenedURL, sr.ShortURL, sr.URL, sr.Short, sr.Data.URL} {
				if val != "" && (strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://")) {
					return strings.TrimSpace(val)
				}
			}
		}
	}

	if strings.HasPrefix(bodyStr, "http://") || strings.HasPrefix(bodyStr, "https://") {
		return bodyStr
	}

	return longURL
}
