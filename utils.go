package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

	if !strings.Contains(splits[0], "/") {
		binary, err := exec.LookPath(splits[0])
		if err != nil {
			return nil, err
		}
		splits[0] = binary
	}

	return splits, nil
}

func remove_duplicates(
	apps []App,
) map[string]App {
	apps_by_name := make(map[string]App)
	apps_by_id := make(map[string]App)
	number_per_name := make(map[string]int)
	for _, app := range apps {
		add := true
		if found, exists := apps_by_id[app.Id]; exists {
			if app.File < found.File {
				delete(apps_by_name, found.Name)
				number_per_name[found.Name] -= 1
			} else {
				add = false
			}
		}
		if add {
			if number_per_name[app.Name] == 0 {
				apps_by_name[app.Name] = app
			} else {
				apps_by_name[fmt.Sprintf("%s (%v)", app.Name, number_per_name[app.Name])] = app
			}
			apps_by_id[app.Id] = app
			number_per_name[app.Name] += 1
		}
	}

	return apps_by_name
}

func find_files_with_extension(
	path string,
	extension string,
	wg *sync.WaitGroup,
	files_chan chan<- []string,
) {
	defer wg.Done()

	var files []string

	filepath.WalkDir(path, func(file string, dir_entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if info, err := dir_entry.Info(); err == nil {
			if (info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) &&
				strings.EqualFold(filepath.Ext(info.Name()), extension) {
				files = append(files, file)
			}
		}

		return nil
	})

	files_chan <- files
}
