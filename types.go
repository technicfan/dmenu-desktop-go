package main

type App struct {
	Name string
	File string
	Command string
	Path string
	Id   string
	Dir string
	Number int
}

type Alias struct {
	Command   string `json:"command"`
	IsDesktop bool   `json:"is_desktop"`
}

type Config struct {
	MenuCommand     string           `json:"menu_command"`
	TerminalCommand string           `json:"terminal_command"`
	Aliases         map[string]Alias `json:"aliases"`
	Excludes        []string         `json:"excludes"`
}
