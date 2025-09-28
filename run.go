package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	reserved_chars = " '\\><~|&;$*?#()`\"\t\n"
)

func parse_command(
	command string,
) ([]string, error) {
	if command == "" {
		return nil, errors.New("Command is empty")
	}

	var splits []string
	var builder strings.Builder
	quoted := false
	escaped := false
	skip := false
	ignore_quotes := false
	for i, r := range command {
		if skip {
			skip = false
			continue
		}
		if i == len(command)-1 {
			if !(quoted && !escaped && r == '"') {
				builder.WriteRune(r)
			}
			splits = append(splits, builder.String())
			continue
		}
		if r == '"' &&
			!escaped &&
			(command[i+1] == ' ' || (i != 0 && command[i-1] == ' ')) &&
			!ignore_quotes {
			quoted = !quoted
			continue
		} else if r == '"' && !escaped {
			ignore_quotes = !ignore_quotes
			continue
		}
		if !escaped && r == '\\' {
			skip = true
			escaped = true
			continue
		}
		if !quoted && !escaped && r == ' ' {
			splits = append(splits, builder.String())
			builder.Reset()
			continue
		}
		if !quoted && escaped && r == ' ' {
			escaped = false
			builder.WriteRune(r)
			continue
		}
		if escaped && r == '\\' && (!quoted || command[i+1] == '\\') {
			skip = true
			escaped = false
			builder.WriteRune(r)
			continue
		}
		if !escaped && !quoted && strings.Contains(reserved_chars, string(r)) {
			return nil, errors.New("Malformed Exec key")
		}
		builder.WriteRune(r)
	}

	binary, err := exec.LookPath(splits[0])
	if err != nil {
		return nil, err
	}
	splits[0] = binary

	return splits, nil
}

func run_desktop(
	path string,
	config Config,
) error {
	desktop_entry, err := get_desktop_string(path)
	if err != nil {
		return err
	}
	if desktop_entry == "" {
		return fmt.Errorf("%s is invalid", path)
	}

	re := regexp.MustCompile("(?m)^Exec=.*")
	matches := re.FindStringSubmatch(desktop_entry)
	if len(matches) == 0 {
		return fmt.Errorf("%s has no Exec key", path)
	}
	command_string := strings.Replace(matches[0], "Exec=", "", 1)
	re = regexp.MustCompile("( )*%.( )*")
	command_string = re.ReplaceAllString(command_string, "")

	var command []string
	if strings.Contains(desktop_entry, "Terminal=true") {
		command = strings.Split(config.TerminalCommand, " ")
		command = append(command, command_string)
	} else {
		command, err = parse_command(command_string)
		if err != nil {
			return err
		}
	}

	re = regexp.MustCompile("(?m)^Path=.*")
	matches = re.FindStringSubmatch(desktop_entry)
	if len(matches) != 0 {
		os.Chdir(strings.Replace(matches[0], "Path=", "", 1))
	}

	err = unix.Exec(command[0], command, os.Environ())

	return err
}

func run(
	name string,
	config Config,
	apps map[string]App,
) error {
	if alias, exists := config.Aliases[name]; exists {
		if !alias.IsDesktop {
			command, err := parse_command(alias.Command)
			if err != nil {
				return err
			}
			err = unix.Exec(command[0], command, os.Environ())

			return err
		} else {
			var err error

			if app, exists := apps[alias.Command]; exists {
				err = run_desktop(app.File, config)
			} else if strings.HasSuffix(alias.Command, ".desktop") {
				err = run_desktop(alias.Command, config)
			}

			return err
		}
	}

	if app, exists := apps[name]; exists {
		err := run_desktop(app.File, config)

		return err
	}

	command, err := parse_command(name)
	if err != nil {
		return err
	}
	err = unix.Exec(command[0], command, os.Environ())

	return err
}
