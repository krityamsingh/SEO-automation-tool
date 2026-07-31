package colly

import (
	"net/url"
	"time"
)

type CollectorOption func(*Collector)

type Response struct {
	StatusCode int
	Body       []byte
	Request    *Request
}

type Request struct {
	URL *url.URL
}

func (r *Request) Visit(URL string) error {
	return nil
}

func (r *Request) AbsoluteURL(path string) string {
	if r.URL != nil {
		u, err := r.URL.Parse(path)
		if err == nil {
			return u.String()
		}
	}
	return path
}

type HTMLElement struct {
	Name       string
	Text       string
	Request    *Request
	attributes map[string]string
}

func (h *HTMLElement) Attr(k string) string {
	if h.attributes != nil {
		return h.attributes[k]
	}
	return ""
}

func (h *HTMLElement) ChildText(goquery string) string {
	return ""
}

func (h *HTMLElement) ChildAttr(goquery, attrName string) string {
	return ""
}

type LimitRule struct {
	DomainRegexp string
	DomainGlob   string
	Delay        time.Duration
	Parallelism  int
}

type Collector struct {
	UserAgent      string
	MaxDepth       int
	AllowedDomains []string
	callbacks      map[string][]func(*HTMLElement)
}

func UserAgent(ua string) CollectorOption {
	return func(c *Collector) {
		c.UserAgent = ua
	}
}

func AllowedDomains(domains ...string) CollectorOption {
	return func(c *Collector) {
		c.AllowedDomains = domains
	}
}

func MaxDepth(depth int) CollectorOption {
	return func(c *Collector) {
		c.MaxDepth = depth
	}
}

func Async(a bool) CollectorOption {
	return func(c *Collector) {}
}

func NewCollector(options ...CollectorOption) *Collector {
	c := &Collector{
		callbacks: make(map[string][]func(*HTMLElement)),
	}
	for _, opt := range options {
		opt(c)
	}
	return c
}

func (c *Collector) Limit(rule *LimitRule) error {
	return nil
}

func (c *Collector) OnHTML(selector string, f func(*HTMLElement)) {
	c.callbacks[selector] = append(c.callbacks[selector], f)
}

func (c *Collector) OnRequest(f func(*Request)) {}

func (c *Collector) OnError(f func(*Response, error)) {}

func (c *Collector) Wait() {}

func (c *Collector) Visit(URL string) error {
	return nil
}

func (c *Collector) Clone() *Collector {
	c2 := &Collector{
		UserAgent:      c.UserAgent,
		MaxDepth:       c.MaxDepth,
		AllowedDomains: append([]string{}, c.AllowedDomains...),
		callbacks:      make(map[string][]func(*HTMLElement)),
	}
	for k, v := range c.callbacks {
		c2.callbacks[k] = append([]func(*HTMLElement){}, v...)
	}
	return c2
}
