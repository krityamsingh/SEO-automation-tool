package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

type Crawler struct {
	userAgent     string
	delay         time.Duration
	mu            sync.Mutex // guards concurrent Crawl/GetPage calls
	AllowLoopback bool       // Exported for testing with local httptest.Server
}

type HeadingNode struct {
	Tag  string `json:"tag"`  // "h1", "h2", "h3", "h4", "h5", "h6"
	Text string `json:"text"`
}

type LinkInfo struct {
	URL        string `json:"url"`
	AnchorText string `json:"anchor_text"`
	IsExternal bool   `json:"is_external"`
	StatusCode int    `json:"status_code"`
	IsBroken   bool   `json:"is_broken"`
}

type PageData struct {
	URL            string             `json:"url"`
	StatusCode     int                `json:"status_code"`
	Title          string             `json:"title"`
	Description    string             `json:"description"`
	Canonical      string             `json:"canonical"`
	OGTags         map[string]string  `json:"og_tags"`
	Headings       []HeadingNode      `json:"headings"`
	Links          []LinkInfo         `json:"links"`
	Images         []ImageData        `json:"images"`
	MetaTags       map[string]string  `json:"meta_tags"`
	BodyText       string             `json:"body_text"`
	Text           string             `json:"text"`
	WordCount      int                `json:"word_count"`
	KeywordDensity map[string]float64 `json:"keyword_density"`
}

type ImageData struct {
	Src string
	Alt string
}

func New(userAgent string, delay time.Duration) *Crawler {
	return &Crawler{
		userAgent:     userAgent,
		delay:         delay,
		AllowLoopback: false,
	}
}

// IsSafeOutboundURL validates whether a target URL is safe against SSRF attacks.
// Blocks loopback IPs (127.0.0.1, ::1, 127.0.0.0/8), localhost, 0.0.0.0,
// private IPs (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16), cloud metadata IPs (169.254.169.254, 169.254.0.0/16),
// multicast, and link-local IPs. Hostnames undergo DNS resolution and resolved IPs are validated.
func IsSafeOutboundURL(rawURL string) bool {
	return isSafeOutboundURLWithOption(rawURL, false)
}

func isSafeOutboundURL(rawURL string) bool {
	return isSafeOutboundURLWithOption(rawURL, false)
}

func (cr *Crawler) isURLSafe(rawURL string) bool {
	return isSafeOutboundURLWithOption(rawURL, cr.AllowLoopback)
}

func isSafeOutboundURLWithOption(rawURL string, allowLoopback bool) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	lowerHost := strings.ToLower(host)

	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
		if !allowLoopback {
			return false
		}
	}

	if lowerHost == "0.0.0.0" {
		return false
	}

	ip := net.ParseIP(host)
	if ip != nil {
		return !isIPBlocked(ip, allowLoopback)
	}

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, resolvedIP := range ips {
		if isIPBlocked(resolvedIP, allowLoopback) {
			return false
		}
	}

	return true
}

func isIPBlocked(ip net.IP, allowLoopback bool) bool {
	if ip == nil {
		return true
	}

	if ip.IsLoopback() {
		if !allowLoopback {
			return true
		}
	}

	if ip.IsUnspecified() {
		return true
	}

	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}

	if isRFC1918IP(ip) {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 127 {
			if !allowLoopback {
				return true
			}
		}
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		if ip4[0] == 0 && ip4[1] == 0 && ip4[2] == 0 && ip4[3] == 0 {
			return true
		}
	}

	return false
}

func isRFC1918IP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	return false
}

// CrawlURL fetches and parses a target URL into PageData using safe HTTP execution.
func (cr *Crawler) CrawlURL(ctx context.Context, targetURL string) (*PageData, error) {
	if !cr.isURLSafe(targetURL) {
		return nil, fmt.Errorf("SSRF guard: unsafe or invalid outbound URL %s", targetURL)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	ua := cr.userAgent
	if ua == "" {
		ua = "AEO-GEO-SEO-Agent/1.0"
	}
	req.Header.Set("User-Agent", ua)

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	parsedTarget, _ := url.Parse(targetURL)

	pageData := &PageData{
		URL:            targetURL,
		StatusCode:     resp.StatusCode,
		OGTags:         make(map[string]string),
		MetaTags:       make(map[string]string),
		Headings:       make([]HeadingNode, 0),
		Links:          make([]LinkInfo, 0),
		Images:         make([]ImageData, 0),
		KeywordDensity: make(map[string]float64),
	}

	pageData.Title = strings.TrimSpace(doc.Find("title").First().Text())

	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		property, _ := s.Attr("property")
		content, _ := s.Attr("content")

		if strings.EqualFold(name, "description") {
			pageData.Description = strings.TrimSpace(content)
		}

		if name != "" {
			pageData.MetaTags[strings.ToLower(name)] = content
		}

		if strings.HasPrefix(strings.ToLower(property), "og:") {
			pageData.OGTags[strings.ToLower(property)] = content
		}
	})

	doc.Find("link").Each(func(_ int, s *goquery.Selection) {
		rel, _ := s.Attr("rel")
		href, _ := s.Attr("href")
		if strings.EqualFold(rel, "canonical") && href != "" {
			if parsedTarget != nil {
				if u, err := parsedTarget.Parse(href); err == nil {
					pageData.Canonical = u.String()
				} else {
					pageData.Canonical = href
				}
			} else {
				pageData.Canonical = href
			}
		}
	})

	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(_ int, s *goquery.Selection) {
		tag := strings.ToLower(goquery.NodeName(s))
		text := strings.TrimSpace(s.Text())
		pageData.Headings = append(pageData.Headings, HeadingNode{
			Tag:  tag,
			Text: text,
		})
	})

	bodySelection := doc.Find("body").Clone()
	bodySelection.Find("script, style, noscript, svg").Remove()
	bodyText := strings.TrimSpace(bodySelection.Text())
	pageData.BodyText = bodyText
	pageData.Text = bodyText

	words := cleanAndTokenize(bodyText)
	pageData.WordCount = len(words)
	pageData.KeywordDensity = computeKeywordDensity(words)

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		anchorText := strings.TrimSpace(s.Text())
		href = strings.TrimSpace(href)

		lowerHref := strings.ToLower(href)
		if href == "" ||
			strings.HasPrefix(lowerHref, "#") ||
			strings.HasPrefix(lowerHref, "javascript:") ||
			strings.HasPrefix(lowerHref, "mailto:") ||
			strings.HasPrefix(lowerHref, "tel:") ||
			strings.HasPrefix(lowerHref, "data:") {
			return
		}

		if u, err := url.Parse(href); err == nil && u.Scheme != "" {
			scheme := strings.ToLower(u.Scheme)
			if scheme != "http" && scheme != "https" {
				return
			}
		}

		resolvedURL := href
		isExternal := false

		if parsedTarget != nil {
			u, err := parsedTarget.Parse(href)
			if err == nil {
				resolvedURL = u.String()
				if u.Hostname() != "" && !strings.EqualFold(u.Hostname(), parsedTarget.Hostname()) {
					isExternal = true
				}
			}
		}

		pageData.Links = append(pageData.Links, LinkInfo{
			URL:        resolvedURL,
			AnchorText: anchorText,
			IsExternal: isExternal,
			StatusCode: 0,
			IsBroken:   false,
		})
	})

	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		alt, _ := s.Attr("alt")
		if parsedTarget != nil {
			if u, err := parsedTarget.Parse(src); err == nil {
				src = u.String()
			}
		}
		pageData.Images = append(pageData.Images, ImageData{
			Src: src,
			Alt: alt,
		})
	})

	return pageData, nil
}

// CheckLinksConcurrently checks the status of extracted links concurrently using worker pools.
func (cr *Crawler) CheckLinksConcurrently(ctx context.Context, links []LinkInfo) []LinkInfo {
	if len(links) == 0 {
		return links
	}

	result := make([]LinkInfo, len(links))
	copy(result, links)

	type checkJob struct {
		index int
		link  LinkInfo
	}

	jobs := make(chan checkJob, len(links))
	results := make(chan checkJob, len(links))

	workerCount := 10
	if len(links) < workerCount {
		workerCount = len(links)
	}

	ua := cr.userAgent
	if ua == "" {
		ua = "AEO-GEO-SEO-Agent/1.0"
	}

	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{
				Timeout: 5 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					if len(via) >= 10 {
						return fmt.Errorf("too many redirects")
					}
					return nil
				},
			}

			for job := range jobs {
				select {
				case <-ctx.Done():
					job.link.IsBroken = true
					results <- job
					continue
				default:
				}

				if !cr.isURLSafe(job.link.URL) {
					job.link.IsBroken = true
					job.link.StatusCode = 0
					results <- job
					continue
				}

				req, err := http.NewRequestWithContext(ctx, "HEAD", job.link.URL, nil)
				if err != nil {
					job.link.IsBroken = true
					job.link.StatusCode = 0
					results <- job
					continue
				}
				req.Header.Set("User-Agent", ua)

				resp, err := client.Do(req)
				if err != nil || resp.StatusCode >= 400 || resp.StatusCode == 405 {
					getReq, getErr := http.NewRequestWithContext(ctx, "GET", job.link.URL, nil)
					if getErr != nil {
						job.link.IsBroken = true
						job.link.StatusCode = 0
						results <- job
						continue
					}
					getReq.Header.Set("User-Agent", ua)

					getResp, getErr := client.Do(getReq)
					if getErr != nil {
						job.link.IsBroken = true
						job.link.StatusCode = 0
					} else {
						getResp.Body.Close()
						job.link.StatusCode = getResp.StatusCode
						if getResp.StatusCode >= 400 {
							job.link.IsBroken = true
						} else {
							job.link.IsBroken = false
						}
					}
				} else {
					resp.Body.Close()
					job.link.StatusCode = resp.StatusCode
					job.link.IsBroken = false
				}

				results <- job
			}
		}()
	}

	for i, l := range result {
		jobs <- checkJob{index: i, link: l}
	}
	close(jobs)

	wg.Wait()
	close(results)

	for res := range results {
		result[res.index] = res.link
	}

	return result
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
	// Concurrent Crawl calls use thread-safe colly collectors

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
	collector.OnHTML("h1, h2, h3, h4, h5, h6", func(e *colly.HTMLElement) {
		resultsMu.Lock()
		if page, ok := results[e.Request.URL.String()]; ok {
			page.Headings = append(page.Headings, HeadingNode{
				Tag:  strings.ToLower(e.Name),
				Text: strings.TrimSpace(e.Text),
			})
		}
		resultsMu.Unlock()
	})
	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Request.AbsoluteURL(e.Attr("href"))
		if link != "" {
			resultsMu.Lock()
			if page, ok := results[e.Request.URL.String()]; ok {
				page.Links = append(page.Links, LinkInfo{
					URL:        link,
					AnchorText: strings.TrimSpace(e.Text),
					IsExternal: false,
				})
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
			page.BodyText = e.Text
			page.Text = e.Text
		}
		resultsMu.Unlock()
	})
	collector.OnRequest(func(r *colly.Request) {
		resultsMu.Lock()
		results[r.URL.String()] = &PageData{
			URL:            r.URL.String(),
			MetaTags:       make(map[string]string),
			OGTags:         make(map[string]string),
			KeywordDensity: make(map[string]float64),
		}
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
	return cr.CrawlURL(context.Background(), urlStr)
}

func (cr *Crawler) ExtractEntities(text string) []string {
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

var stopWordsMap = map[string]bool{
	"a": true, "about": true, "above": true, "after": true, "again": true, "against": true,
	"all": true, "am": true, "an": true, "and": true, "any": true, "are": true, "aren't": true,
	"as": true, "at": true, "be": true, "because": true, "been": true, "before": true,
	"being": true, "below": true, "between": true, "both": true, "but": true, "by": true,
	"can": true, "cannot": true, "could": true, "couldn't": true, "did": true, "didn't": true,
	"do": true, "does": true, "doesn't": true, "doing": true, "don't": true, "down": true,
	"during": true, "each": true, "few": true, "for": true, "from": true, "further": true,
	"had": true, "hadn't": true, "has": true, "hasn't": true, "have": true, "haven't": true,
	"having": true, "he": true, "he'd": true, "he'll": true, "he's": true, "her": true,
	"here": true, "here's": true, "hers": true, "herself": true, "him": true, "himself": true,
	"his": true, "how": true, "how's": true, "i": true, "i'd": true, "i'll": true,
	"i'm": true, "i've": true, "if": true, "in": true, "into": true, "is": true, "isn't": true,
	"it": true, "its": true, "itself": true, "let's": true, "me": true, "more": true,
	"most": true, "mustn't": true, "my": true, "myself": true, "no": true, "nor": true,
	"not": true, "of": true, "off": true, "on": true, "once": true, "only": true,
	"or": true, "other": true, "ought": true, "our": true, "ours": true, "ourselves": true,
	"out": true, "over": true, "own": true, "same": true, "shan't": true, "she": true,
	"she'd": true, "she'll": true, "she's": true, "should": true, "shouldn't": true, "so": true,
	"some": true, "such": true, "than": true, "that": true, "that's": true, "the": true,
	"their": true, "theirs": true, "them": true, "themselves": true, "then": true, "there": true,
	"there's": true, "these": true, "they": true, "they'd": true, "they'll": true, "they're": true,
	"they've": true, "this": true, "those": true, "through": true, "to": true, "too": true,
	"under": true, "until": true, "up": true, "very": true, "was": true, "wasn't": true,
	"we": true, "we'd": true, "we'll": true, "we're": true, "we've": true, "were": true,
	"weren't": true, "what": true, "what's": true, "when": true, "when's": true, "where": true,
	"where's": true, "which": true, "while": true, "who": true, "who's": true, "whom": true,
	"why": true, "why's": true, "with": true, "won't": true, "would": true, "wouldn't": true,
	"you": true, "you'd": true, "you'll": true, "you're": true, "you've": true, "your": true,
	"yours": true, "yourself": true, "yourselves": true,
}

func cleanAndTokenize(text string) []string {
	rawFields := strings.Fields(text)
	words := make([]string, 0, len(rawFields))
	for _, field := range rawFields {
		cleaned := strings.Trim(strings.ToLower(field), ".,!?;:'\"()[]{}\\/|<>-=_+@#$%^&*")
		if len(cleaned) > 0 {
			words = append(words, cleaned)
		}
	}
	return words
}

func computeKeywordDensity(words []string) map[string]float64 {
	densityMap := make(map[string]float64)
	if len(words) == 0 {
		return densityMap
	}

	totalWords := float64(len(words))
	singleCounts := make(map[string]int)

	for _, w := range words {
		if len(w) >= 2 && !stopWordsMap[w] {
			singleCounts[w]++
		}
	}

	for word, count := range singleCounts {
		densityMap[word] = (float64(count) / totalWords) * 100.0
	}

	bigramCounts := make(map[string]int)
	for i := 0; i < len(words)-1; i++ {
		w1, w2 := words[i], words[i+1]
		if len(w1) >= 2 && len(w2) >= 2 && (!stopWordsMap[w1] || !stopWordsMap[w2]) {
			bigram := w1 + " " + w2
			bigramCounts[bigram]++
		}
	}

	for bigram, count := range bigramCounts {
		if count > 1 || len(words) < 50 {
			densityMap[bigram] = (float64(count) / totalWords) * 100.0
		}
	}

	return densityMap
}

