package session

import (
	"time"
)

// SmartTimeStrategy implements time-based session boundaries with activity awareness
type SmartTimeStrategy struct {
	baseTimeout           time.Duration
	activityCheckInterval time.Duration
	extendIfActive        bool
}

func NewSmartTimeStrategy(baseTimeout, activityCheckInterval time.Duration, extendIfActive bool) *SmartTimeStrategy {
	return &SmartTimeStrategy{
		baseTimeout:           baseTimeout,
		activityCheckInterval: activityCheckInterval,
		extendIfActive:        extendIfActive,
	}
}

func (s *SmartTimeStrategy) Name() string {
	return "smart-time"
}

func (s *SmartTimeStrategy) ShouldStartSession(ctx *Context) bool {
	return ctx.SessionID == ""
}

func (s *SmartTimeStrategy) ShouldEndSession(ctx *Context) bool {
	if ctx.SessionID == "" {
		return false
	}

	timeoutExceeded := ctx.TimeSinceLastEvent >= s.baseTimeout
	if !timeoutExceeded {
		return false
	}

	// Don't end if extendIfActive enabled and LLM currently working
	if s.extendIfActive && ctx.IsLLMActive {
		return false
	}

	return true
}
