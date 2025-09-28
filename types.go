package main

type App struct {
	Name string `json:"name"`
	File string `json:"file"`
	Id   string `json:"id"`
}

type Alias struct {
	Command   string `json:"command"`
	IsDesktop bool   `json:"is_desktop"`
}

type Config struct {
	MenuCommand     string           `json:"menu_command"`
	TerminalCommand string           `json:"terminal_command"`
	CacheEnabled    bool             `json:"cache_enabled"`
	Aliases         map[string]Alias `json:"aliases"`
	Excludes        []string         `json:"excludes"`
}
