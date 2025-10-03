package api

import (
	"context"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	hzconfig "github.com/cloudwego/hertz/pkg/common/config"

	"github.com/telnet2/mysql-vfs/internal/fs"
)

// Server wires the Hertz router with file system handlers.
type Server struct {
	fs *fs.Service
	hz *server.Hertz
}

// NewServer creates a Hertz server with minimal REST handlers.
func NewServer(fs *fs.Service, options ...hzconfig.Option) *Server {
	hz := server.New(options...)

	srv := &Server{fs: fs, hz: hz}

	hz.GET("/nodes/*path", srv.handleGetNode)
	hz.GET("/healthz", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	return srv
}

// Run starts the Hertz HTTP server.
func (s *Server) Run() error {
	return s.hz.Run()
}

func (s *Server) handleGetNode(ctx context.Context, c *app.RequestContext) {
	path := string(c.Param("path"))
	node, content, err := s.fs.ReadNode(ctx, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, map[string]any{"node": node, "content": string(content)})
}
