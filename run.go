package main

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func run(
	name string,
	config Config,
	apps map[string]App,
) error {
	var err error
	var path string
	var command []string

	if alias, exists := config.Aliases[name]; exists {
		if !alias.IsDesktop {
			command, err = parse_command(alias.Command)
		} else {
			if app, exists := apps[alias.Command]; exists {
				command, path, err = get_desktop_command(app.File, config.TerminalCommand)
			} else if strings.HasSuffix(alias.Command, ".desktop") {
				command, path, err = get_desktop_command(alias.Command, config.TerminalCommand)
			} else {
				return errors.New("Invalid alias")
			}
		}
	} else if app, exists := apps[name]; exists {
		command, path, err = get_desktop_command(app.File, config.TerminalCommand)
	} else {
		command, err = parse_command(name)
	}

	if err != nil {
		return err
	}

	if path != "" {
		err = os.Chdir(path)
		if err != nil {
			return err
		}
	}

	err = unix.Exec(command[0], command, os.Environ())

	return err
}
