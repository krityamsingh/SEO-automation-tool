package gin

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const ReleaseMode = "release"

type H map[string]interface{}

func SetMode(mode string) {}

func Recovery() HandlerFunc {
	return func(c *Context) {}
}

func Logger() HandlerFunc {
	return func(c *Context) {}
}

type HandlerFunc func(*Context)

type Context struct {
	Request  *http.Request
	Writer   http.ResponseWriter
	params   map[string]string
	query    map[string]string
	headers  map[string]string
	handlers []HandlerFunc
	index    int
}

func (c *Context) Param(key string) string {
	if c.params != nil {
		return c.params[key]
	}
	return ""
}

func (c *Context) Query(key string) string {
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Query().Get(key)
	}
	if c.query != nil {
		return c.query[key]
	}
	return ""
}

func (c *Context) DefaultQuery(key, defaultValue string) string {
	val := c.Query(key)
	if val == "" {
		return defaultValue
	}
	return val
}

func (c *Context) GetHeader(key string) string {
	if c.Request != nil {
		return c.Request.Header.Get(key)
	}
	if c.headers != nil {
		return c.headers[key]
	}
	return ""
}

func (c *Context) Header(key, value string) {
	if c.Writer != nil {
		c.Writer.Header().Set(key, value)
	}
}

func (c *Context) JSON(code int, obj interface{}) {
	if c.Writer != nil {
		c.Writer.WriteHeader(code)
	}
}

func (c *Context) String(code int, format string, values ...interface{}) {
	if c.Writer != nil {
		c.Writer.WriteHeader(code)
		fmt.Fprintf(c.Writer, format, values...)
	}
}

func (c *Context) Next() {
	c.index++
	for c.index < len(c.handlers) {
		c.handlers[c.index](c)
		c.index++
	}
}

func (c *Context) ShouldBindJSON(obj interface{}) error {
	if c.Request != nil && c.Request.Body != nil {
		return json.NewDecoder(c.Request.Body).Decode(obj)
	}
	return fmt.Errorf("empty request body")
}

func (c *Context) AbortWithStatusJSON(code int, jsonObj interface{}) {
	c.JSON(code, jsonObj)
	c.index = len(c.handlers)
}

type Engine struct {
	middlewares []HandlerFunc
}

func New() *Engine {
	return &Engine{}
}

func Default() *Engine {
	return &Engine{}
}

func (e *Engine) Use(middlewares ...HandlerFunc) {
	e.middlewares = append(e.middlewares, middlewares...)
}

func (e *Engine) GET(path string, handlers ...HandlerFunc) {}

func (e *Engine) POST(path string, handlers ...HandlerFunc) {}

func (e *Engine) Group(relativePath string, handlers ...HandlerFunc) *RouterGroup {
	return &RouterGroup{middlewares: handlers}
}

func (e *Engine) Run(addr string) error {
	return nil
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {}

type RouterGroup struct {
	middlewares []HandlerFunc
}

func (rg *RouterGroup) Use(middlewares ...HandlerFunc) *RouterGroup {
	rg.middlewares = append(rg.middlewares, middlewares...)
	return rg
}

func (rg *RouterGroup) GET(path string, handlers ...HandlerFunc) {}

func (rg *RouterGroup) POST(path string, handlers ...HandlerFunc) {}

