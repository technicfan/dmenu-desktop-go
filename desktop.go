package main

import (
	"fmt"
	"io"
	"log"
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
	path string,
	terminal_command string,
) ([]string, string, error) {
	var run_path string

	desktop_entry, err := get_desktop_string(path)
	if err != nil {
		return nil, "", err
	}
	if desktop_entry == "" {
		return nil, "", fmt.Errorf("%s is invalid", path)
	}

	re := regexp.MustCompile("(?m)^Exec=.*")
	matches := re.FindStringSubmatch(desktop_entry)
	if len(matches) == 0 {
		return nil, "", fmt.Errorf("%s has no Exec key", path)
	}
	command_string := strings.Replace(matches[0], "Exec=", "", 1)
	re = regexp.MustCompile("( )*%.( )*")
	command_string = re.ReplaceAllString(command_string, "")

	var command []string
	if strings.Contains(desktop_entry, "Terminal=true") {
		command = strings.Split(terminal_command, " ")
		command = append(command, command_string)
	} else {
		command, err = parse_command(command_string)
		if err != nil {
			return nil, "", err
		}
	}

	re = regexp.MustCompile("(?m)^Path=.*")
	matches = re.FindStringSubmatch(desktop_entry)
	if len(matches) != 0 {
		run_path = strings.Replace(matches[0], "Path=", "", 1)
	}

	return command, run_path, nil
}

func get_desktop_details(
	path string,
	lang string,
	wg *sync.WaitGroup,
	apps chan<- App,
) {
	defer wg.Done()

	desktop_entry, err := get_desktop_string(path)
	if err != nil {
		log.Fatalf("Failed to read %s: %s", path, err.Error())
	}
	if desktop_entry == "" {
		return
	}

	re := regexp.MustCompile("(?m)^(NoDisplay|Hidden)=true$")

	if re.MatchString(desktop_entry) ||
		!regexp.MustCompile("(?m)^Type=Application").MatchString(desktop_entry) {
		return
	}

	id := strings.ReplaceAll(filepath.Base(path), ".desktop", "")

	re = regexp.MustCompile(fmt.Sprintf(`(?m)^Name\[%s\]=.*`, lang))
	matches := re.FindStringSubmatch(desktop_entry)
	if len(matches) > 0 {
		apps <- App{
			strings.Replace(matches[0], fmt.Sprintf("Name[%s]=", lang), "", 1),
			path,
			id,
		}
	} else {
		re = regexp.MustCompile("(?m)^Name=.*")
		matches = re.FindStringSubmatch(desktop_entry)
		if len(matches) > 0 {
			apps <- App{
				strings.Replace(matches[0], "Name=", "", 1),
				path,
				id,
			}
		}
	}
}
