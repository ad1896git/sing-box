package boxapi

import (
	"context"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
)

type SbV2rayServer struct {
	ss *SbStatsService
}

func NewSbV2rayServer(options option.V2RayStatsServiceOptions) *SbV2rayServer {
	return &SbV2rayServer{
		ss: NewSbStatsService(options),
	}
}

func (s *SbV2rayServer) Start() error { return nil }
func (s *SbV2rayServer) Close() error { return nil }

// StatsService returns a nil interface, not a typed nil, when stats are disabled.
func (s *SbV2rayServer) StatsService() adapter.V2RayStatsService {
	if s.ss == nil {
		return nil
	}
	return s.ss
}

// NekoRay style API

func (s *SbV2rayServer) QueryStats(name string) int64 {
	if s.ss == nil {
		return 0
	}
	value, err := s.ss.GetStats(context.TODO(), name, true)
	if err == nil {
		return value
	}
	return 0
}
