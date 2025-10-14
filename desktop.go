package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

func get_desktop_string(
	path string,
) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	byte_data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`(?s)(?m)^\[Desktop Entry\](.*?)(^\[|\z)`)
	matches := re.FindStringSubmatch(string(byte_data))
	if len(matches) == 0 {
		return "", nil
	}

	return matches[0], nil
}

func get_desktop_command(
	app App,
) ([]string, string, error) {
	re := regexp.MustCompile("(( )*%[fFuUi]( )*|( )*@@u %u @@)")
	command_string := re.ReplaceAllString(app.Command, "")
	command_string = strings.ReplaceAll(command_string, "%%", "%")
	command_string = strings.ReplaceAll(command_string, "%k", app.File)

	command, err := parse_command(command_string)
	if err != nil {
		return nil, "", err
	}

	return command, app.Path, nil
}

func get_app(
	path string,
	lang string,
	terminal_command string,
	regexp_id *regexp.Regexp,
) (App, error) {
	desktop_entry, err := get_desktop_string(path)
	if err != nil {
		return App{}, fmt.Errorf("Failed to read %s: %s", path, err.Error())
	}
	if desktop_entry == "" {
		return App{}, errors.New("Invalid desktop entry")
	}

	re := regexp.MustCompile("(?m)^(NoDisplay|Hidden)=true$")

	if re.MatchString(desktop_entry) ||
		!regexp.MustCompile("(?m)^Type=Application").MatchString(desktop_entry) {
		return App{}, errors.New("No visible application")
	}

	re = regexp.MustCompile("(?m)^Exec=.*")
	matches := re.FindStringSubmatch(desktop_entry)
	if len(matches) == 0 {
		return App{}, fmt.Errorf("%s has no Exec key", path)
	}
	command_string := strings.Replace(matches[0], "Exec=", "", 1)

	if strings.Contains(desktop_entry, "Terminal=true") {
		command_string = fmt.Sprintf("%s %s", terminal_command, command_string)
	}

	var run_path string
	re = regexp.MustCompile("(?m)^Path=.*")
	matches = re.FindStringSubmatch(desktop_entry)
	if len(matches) != 0 {
		run_path = strings.Replace(matches[0], "Path=", "", 1)
	}

	id := strings.ReplaceAll(
		regexp.MustCompile(".desktop$").ReplaceAllString(regexp_id.Split(path, 2)[1], ""),
		"/",
		"-",
	)

	re = regexp.MustCompile(fmt.Sprintf(`(?m)^Name\[%s\]=.*`, lang))
	matches = re.FindStringSubmatch(desktop_entry)
	if len(matches) > 0 {
		return App{
			strings.Replace(matches[0], fmt.Sprintf("Name[%s]=", lang), "", 1),
			path,
			command_string,
			run_path,
			id,
			filepath.Dir(path),
			0,
		}, nil
	} else {
		re = regexp.MustCompile("(?m)^Name=.*")
		matches = re.FindStringSubmatch(desktop_entry)
		if len(matches) > 0 {
			return App{
				strings.Replace(matches[0], "Name=", "", 1),
				path,
				command_string,
				run_path,
				id,
				filepath.Dir(path),
				0,
			}, nil
		}
	}

	return App{}, errors.New("Invalid desktop entry")
}

func get_app_async(
	path string,
	lang string,
	terminal_command string,
	regexp_id *regexp.Regexp,
	wg *sync.WaitGroup,
	apps chan<- App,
) {
	defer wg.Done()

	app, err := get_app(path, lang, terminal_command, regexp_id)
	if err == nil {
		apps <- app
	}
}
