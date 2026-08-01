package crawler

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
)

type Crawler struct {
	userAgent string
	delay     time.Duration
	mu        sync.Mutex // guards concurrent Crawl/GetPage calls
}

type PageData struct {
	URL         string
	Title       string
	Description string
	Headings    []string
	Text        string
	Links       []string
	Images      []ImageData
	MetaTags    map[string]string
}

type ImageData struct {
	Src string
	Alt string
}

func New(userAgent string, delay time.Duration) *Crawler {
	return &Crawler{
		userAgent: userAgent,
		delay:     delay,
	}
}

// newCollector creates a fresh, unshared Colly collector with the crawler's
// user-agent/delay settings. Each crawl gets its own collector + results map.
func (cr *Crawler) newCollector() *colly.Collector {
	c := colly.NewCollector(
		colly.UserAgent(cr.userAgent),
		colly.MaxDepth(2),
		colly.Async(true),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		Delay:       cr.delay,
	})
	return c
}

func (cr *Crawler) Crawl(targetURL string, maxDepth int, maxPages int) (map[string]*PageData, error) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	collector := cr.newCollector()
	collector.MaxDepth = maxDepth
	collector.AllowedDomains = []string{parsedURL.Host}

	results := make(map[string]*PageData)
	var resultsMu sync.Mutex

	collector.OnHTML("title", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Title = e.Text
		}
		resultsMu.Unlock()
	})
	collector.OnHTML("meta[name=description]", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Description = e.Attr("content")
		}
		resultsMu.Unlock()
	})
	collector.OnHTML("h1, h2, h3", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Headings = append(page.Headings, fmt.Sprintf("%s: %s", e.Name, e.Text))
		}
		resultsMu.Unlock()
	})
	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Request.AbsoluteURL(e.Attr("href"))
		if link != "" {
			resultsMu.Lock()
			if page, ok := results[e.Request.URL.String()]; ok {
				page.Links = append(page.Links, link)
			}
			resultsMu.Unlock()
		}
	})
	collector.OnHTML("img", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Images = append(page.Images, ImageData{
				Src: e.Request.AbsoluteURL(e.Attr("src")),
				Alt: e.Attr("alt"),
			})
		}
		resultsMu.Unlock()
	})
	collector.OnHTML("body", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Text = e.Text
		}
		resultsMu.Unlock()
	})
	collector.OnRequest(func(r *colly.Request) {
		resultsMu.Lock()
		results[r.URL.String()] = &PageData{URL: r.URL.String(), MetaTags: make(map[string]string)}
		resultsMu.Unlock()
		slog.Debug("crawling", "url", r.URL.String())
	})

	err = collector.Visit(targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to start crawl: %w", err)
	}

	collector.Wait()

	if maxPages > 0 && len(results) > maxPages {
		trimmed := make(map[string]*PageData, maxPages)
		i := 0
		for k, v := range results {
			if i >= maxPages {
				break
			}
			trimmed[k] = v
			i++
		}
		results = trimmed
	}

	slog.Info("crawl complete", "url", targetURL, "pages", len(results))
	return results, nil
}

func (cr *Crawler) GetPage(urlStr string) (*PageData, error) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	collector := cr.newCollector()
	collector.MaxDepth = 0
	collector.AllowedDomains = []string{}

	results := make(map[string]*PageData)
	var resultsMu sync.Mutex

	collector.OnHTML("title", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Title = e.Text
		}
		resultsMu.Unlock()
	})
	collector.OnHTML("body", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Text = e.Text
		}
		resultsMu.Unlock()
	})
	collector.OnRequest(func(r *colly.Request) {
		resultsMu.Lock()
		results[r.URL.String()] = &PageData{URL: r.URL.String(), MetaTags: make(map[string]string)}
		resultsMu.Unlock()
	})

	err := collector.Visit(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	collector.Wait()

	resultsMu.Lock()
	page, ok := results[urlStr]
	resultsMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("page not found: %s", urlStr)
	}
	return page, nil
}

func (cr *Crawler) ExtractEntities(text string) []string {
	// Simple entity extraction - in production, use NLP library
	words := strings.Fields(text)
	entityMap := make(map[string]bool)

	for i := 0; i < len(words)-1; i++ {
		word := words[i]
		if len(word) > 4 && strings.ToUpper(word[:1]) == word[:1] {
			entityMap[word] = true
		}
	}

	entities := make([]string, 0, len(entityMap))
	for e := range entityMap {
		entities = append(entities, e)
	}
	return entities
}
