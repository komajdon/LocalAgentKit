package tools

import (
	"time"

	"agent-gui/domain"
)

type timeTool struct{}

func NewGetTime() domain.Tool { return &timeTool{} }

func (t *timeTool) Name() string                        { return "get_time" }
func (t *timeTool) Description() string                 { return "Get the current date and time" }
func (t *timeTool) Parameters() string                  { return `{}` }
func (t *timeTool) Execute(_ map[string]string) string  { return time.Now().Format("2006-01-02 15:04:05 MST") }
