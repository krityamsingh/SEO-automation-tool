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
	userAgent  string
	delay      time.Duration
	colly      *colly.Collector
	results    map[string]*PageData
	mu         sync.RWMutex
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
	Src  string
	Alt  string
}

func New(userAgent string, delay time.Duration) *Crawler {
	c := colly.NewCollector(
		colly.UserAgent(userAgent),
		colly.AllowedDomains(),
		colly.MaxDepth(2),
		colly.Async(true),
	)
	
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		Delay:       delay,
	})
	
	cr := &Crawler{
		userAgent: userAgent,
		delay:     delay,
		colly:     c,
		results:   make(map[string]*PageData),
	}
	
	c.OnHTML("title", func(e *colly.HTMLElement) {
		cr.mu.Lock()
		if page, ok := cr.results[e.Request.URL.String()]; ok {
			page.Title = e.Text
		}
		cr.mu.Unlock()
	})
	
	c.OnHTML("meta[name=description]", func(e *colly.HTMLElement) {
		cr.mu.Lock()
		if page, ok := cr.results[e.Request.URL.String()]; ok {
			page.Description = e.Attr("content")
		}
		cr.mu.Unlock()
	})
	
	c.OnHTML("h1, h2, h3", func(e *colly.HTMLElement) {
		cr.mu.Lock()
		if page, ok := cr.results[e.Request.URL.String()]; ok {
			page.Headings = append(page.Headings, fmt.Sprintf("%s: %s", e.Name, e.Text))
		}
		cr.mu.Unlock()
	})
	
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Request.AbsoluteURL(e.Attr("href"))
		if link != "" {
			cr.mu.Lock()
			if page, ok := cr.results[e.Request.URL.String()]; ok {
				page.Links = append(page.Links, link)
			}
			cr.mu.Unlock()
		}
	})
	
	c.OnHTML("img", func(e *colly.HTMLElement) {
		cr.mu.Lock()
		if page, ok := cr.results[e.Request.URL.String()]; ok {
			page.Images = append(page.Images, ImageData{
				Src: e.Request.AbsoluteURL(e.Attr("src")),
				Alt: e.Attr("alt"),
			})
		}
		cr.mu.Unlock()
	})
	
	c.OnHTML("body", func(e *colly.HTMLElement) {
		cr.mu.Lock()
		if page, ok := cr.results[e.Request.URL.String()]; ok {
			page.Text = e.Text
		}
		cr.mu.Unlock()
	})
	
	c.OnRequest(func(r *colly.Request) {
		cr.mu.Lock()
		cr.results[r.URL.String()] = &PageData{URL: r.URL.String(), MetaTags: make(map[string]string)}
		cr.mu.Unlock()
		slog.Debug("crawling", "url", r.URL.String())
	})
	
	c.OnError(func(r *colly.Response, err error) {
		slog.Warn("crawl error", "url", r.Request.URL, "error", err)
	})
	
	return cr
}

func (cr *Crawler) Crawl(targetURL string, maxDepth int, maxPages int) (map[string]*PageData, error) {
	cr.results = make(map[string]*PageData)
	cr.colly.MaxDepth = maxDepth
	
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	
	cr.colly.AllowedDomains = []string{parsedURL.Host}
	
	err = cr.colly.Visit(targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to start crawl: %w", err)
	}
	
	cr.colly.Wait()
	
	cr.mu.RLock()
	results := make(map[string]*PageData)
	for k, v := range cr.results {
		results[k] = v
		if len(results) >= maxPages {
			break
		}
	}
	cr.mu.RUnlock()
	
	slog.Info("crawl complete", "url", targetURL, "pages", len(results))
	return results, nil
}

func (cr *Crawler) GetPage(url string) (*PageData, error) {
	cr.results = make(map[string]*PageData)
	cr.colly.MaxDepth = 0
	
	cr.colly.AllowedDomains = []string{}
	
	err := cr.colly.Visit(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	
	cr.colly.Wait()
	
	cr.mu.RLock()
	page, ok := cr.results[url]
	cr.mu.RUnlock()
	
	if !ok {
		return nil, fmt.Errorf("page not found: %s", url)
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
