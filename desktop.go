package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

func get_desktop_command(
	app App,
) ([]string, string, error) {
	re := regexp.MustCompile("( )*(@@u?)?( )*%[fFuUi]( )*(@@)?")
	command_string := re.ReplaceAllString(app.Command, "")
	command_string = strings.ReplaceAll(command_string, "%%", "%")
	command_string = strings.ReplaceAll(command_string, "%k", app.File)

	command, err := parse_command(strings.TrimSpace(command_string))
	if err != nil {
		return nil, "", err
	}

	return command, app.Path, nil
}

func get_app(
	path string,
	localized_name_key string,
	terminal_command string,
	dirs []string,
) (App, error) {
	file, err := os.Open(path)
	if err != nil {
		return App{}, fmt.Errorf("Failed to read %s: %s", path, err.Error())
	}
	defer file.Close()

	app := App{}
	var exe string
	var is_entry, is_app, terminal bool

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		} else if line == "[Desktop Entry]" {
			is_entry = true
			continue
		}
		if !is_entry {
			return App{}, errors.New("Invalid desktop entry")
		}
		if strings.HasPrefix(line, "[") {
			break
		}
		switch {
		case line == "NoDisplay=true" || line == "Hidden=true":
			return App{}, errors.New("No visible application")
		case line == "Terminal=true":
			terminal = true
		case line == "Type=Application":
			is_app = true
		case strings.HasPrefix(line, "Exec="):
			exe = strings.Replace(line, "Exec=", "", 1)
		case strings.HasPrefix(line, "Path="):
			app.Path = strings.Replace(line, "Path=", "", 1)
		case strings.HasPrefix(line, localized_name_key):
			app.Name = strings.Replace(line, localized_name_key, "", 1)
		case strings.HasPrefix(line, "Name=") && app.Name == "":
			app.Name = strings.Replace(line, "Name=", "", 1)
		}
	}
	if terminal {
		exe = fmt.Sprintf("%s %s", terminal_command, exe)
	}
	app.Dir = filepath.Dir(path)
	app.Command = exe
	app.File = path
	for _, dir := range dirs {
		if strings.HasPrefix(path, dir) {
			app.Id = strings.ReplaceAll(strings.Replace(path, dir, "", 1), "/", "-")
			break
		}
	}

	if is_app && app.Name != "" && app.Command != "" {
		return app, nil
	}

	return App{}, errors.New("Invalid desktop entry")
}

func get_app_async(
	path string,
	localized_name_key string,
	terminal_command string,
	dirs []string,
	wg *sync.WaitGroup,
	apps chan<- App,
) {
	defer wg.Done()

	app, err := get_app(path, localized_name_key, terminal_command, dirs)
	if err == nil {
		apps <- app
	}
}
