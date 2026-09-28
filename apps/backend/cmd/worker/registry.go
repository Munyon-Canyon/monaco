package main

import (
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

type consumerSet []func(config.Config) []bus.Consumer

var registered consumerSet

func (s *consumerSet) add(source func(config.Config) []bus.Consumer) {
	*s = append(*s, source)
}

func (s consumerSet) consumers(cfg config.Config) []bus.Consumer {
	all := make([]bus.Consumer, 0, len(s))
	for _, source := range s {
		all = append(all, source(cfg)...)
	}
	return all
}
