package gin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
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
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Writer.WriteHeader(code)
		json.NewEncoder(c.Writer).Encode(obj)
	}
}

func (c *Context) String(code int, format string, values ...interface{}) {
	if c.Writer != nil {
		if c.Writer.Header().Get("Content-Type") == "" {
			c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		c.Writer.WriteHeader(code)
		if len(values) > 0 {
			fmt.Fprintf(c.Writer, format, values...)
		} else {
			c.Writer.Write([]byte(format))
		}
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

type route struct {
	method   string
	path     string
	handlers []HandlerFunc
}

type Engine struct {
	middlewares []HandlerFunc
	routes      []route
	mu          sync.RWMutex
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

func (e *Engine) GET(path string, handlers ...HandlerFunc) {
	e.addRoute("GET", path, handlers...)
}

func (e *Engine) POST(path string, handlers ...HandlerFunc) {
	e.addRoute("POST", path, handlers...)
}

func (e *Engine) addRoute(method, path string, handlers ...HandlerFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()

	allHandlers := append(append([]HandlerFunc{}, e.middlewares...), handlers...)
	e.routes = append(e.routes, route{
		method:   method,
		path:     path,
		handlers: allHandlers,
	})
}

func (e *Engine) Group(relativePath string, handlers ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		engine:      e,
		basePath:    relativePath,
		middlewares: handlers,
	}
}

func (e *Engine) Run(addr string) error {
	return http.ListenAndServe(addr, e)
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	e.mu.RLock()
	routes := make([]route, len(e.routes))
	copy(routes, e.routes)
	e.mu.RUnlock()

	reqPath := req.URL.Path
	reqMethod := req.Method

	for _, r := range routes {
		if r.method == reqMethod {
			params, matched := matchRoute(r.path, reqPath)
			if matched {
				c := &Context{
					Request:  req,
					Writer:   w,
					params:   params,
					handlers: r.handlers,
					index:    0,
				}
				if len(c.handlers) > 0 {
					c.handlers[0](c)
				}
				return
			}
		}
	}

	http.NotFound(w, req)
}

type RouterGroup struct {
	engine      *Engine
	basePath    string
	middlewares []HandlerFunc
}

func (rg *RouterGroup) Use(middlewares ...HandlerFunc) *RouterGroup {
	rg.middlewares = append(rg.middlewares, middlewares...)
	return rg
}

func (rg *RouterGroup) GET(path string, handlers ...HandlerFunc) {
	fullPath := joinPaths(rg.basePath, path)
	allHandlers := append(append([]HandlerFunc{}, rg.middlewares...), handlers...)
	rg.engine.GET(fullPath, allHandlers...)
}

func (rg *RouterGroup) POST(path string, handlers ...HandlerFunc) {
	fullPath := joinPaths(rg.basePath, path)
	allHandlers := append(append([]HandlerFunc{}, rg.middlewares...), handlers...)
	rg.engine.POST(fullPath, allHandlers...)
}

func joinPaths(base, path string) string {
	if base == "" || base == "/" {
		return path
	}
	if path == "/" || path == "" {
		return base
	}
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(path, "/")
}

func matchRoute(pattern, path string) (map[string]string, bool) {
	if pattern == path {
		return nil, true
	}
	pSegs := strings.Split(strings.Trim(pattern, "/"), "/")
	reqSegs := strings.Split(strings.Trim(path, "/"), "/")

	if len(pSegs) != len(reqSegs) {
		return nil, false
	}

	params := make(map[string]string)
	for i := 0; i < len(pSegs); i++ {
		if strings.HasPrefix(pSegs[i], ":") {
			paramName := strings.TrimPrefix(pSegs[i], ":")
			params[paramName] = reqSegs[i]
		} else if pSegs[i] != reqSegs[i] {
			return nil, false
		}
	}
	return params, true
}
