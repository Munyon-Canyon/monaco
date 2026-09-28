package main

type toolEnv struct {
	environ []string
	wd, exe string
}

var toolFactories = map[string]func(toolEnv) tool{}

func registerTool(name string, factory func(toolEnv) tool) {
	toolFactories[name] = factory
}
