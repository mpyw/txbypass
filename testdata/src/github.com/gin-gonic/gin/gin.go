// Package gin is a stub of github.com/gin-gonic/gin for the analysistest
// fixtures.
//
// The signatures are those of github.com/gin-gonic/gin v1.10.0. Only what the
// fixtures use is kept, and the bodies do nothing. MIT License:
// https://github.com/gin-gonic/gin/blob/v1.10.0/LICENSE
package gin

import (
	"net/http"
	"time"
)

// H is a map for a JSON body.
type H map[string]any

// Context is a request's context. It implements context.Context.
type Context struct {
	Request *http.Request
}

func (c *Context) Deadline() (deadline time.Time, ok bool) { return }

func (c *Context) Done() <-chan struct{} { return nil }

func (c *Context) Err() error { return nil }

func (c *Context) Value(key any) any { return nil }

func (c *Context) Param(key string) string { return "" }

func (c *Context) JSON(code int, obj any) {}
