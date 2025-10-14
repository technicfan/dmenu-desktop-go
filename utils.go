package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	for i, r := range command {
		if skip {
			skip = false
			continue
		}

		switch {
		case r == '"' && !escaped:
			quoted = !quoted
		case r == ' ' && !quoted:
			if escaped {
				escaped = false
				builder.WriteRune(r)
			} else {
				splits = append(splits, builder.String())
				builder.Reset()
			}
		case r == '\\':
			if escaped && (!quoted || command[i+1] == '\\') {
				skip = true
				escaped = false
				builder.WriteRune(r)
			} else if !escaped {
				skip = true
				escaped = true
			}
		default:
			if !escaped && !quoted && strings.Contains(reserved_chars, string(r)) {
				return nil, fmt.Errorf("Unescaped %s at position %v in Exec key", string(r), i)
			} else {
				builder.WriteRune(r)
			}
		}
	}
	
	splits = append(splits, builder.String())

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
	dirs []string,
) map[string]App {
	apps_by_name := make(map[string]App)
	apps_by_id := make(map[string]App)
	number_per_name := make(map[string]int)
	for _, app := range apps {
		add := true
		if found, exists := apps_by_id[app.Id]; exists {
			if slices.Index(dirs, app.Dir) < slices.Index(dirs, found.Dir) {
				if found.Number == 0 {
					delete(apps_by_name, found.Name)
				} else {
					delete(apps_by_name, fmt.Sprintf("%s (%v)", found.Name, found.Number))
				}
				if number_per_name[found.Name] != 0 {
					number_per_name[found.Name] -= 1
				}
			} else {
				add = false
			}
		}
		if add {
			if number_per_name[app.Name] == 0 {
				apps_by_name[app.Name] = app
			} else {
				apps_by_name[fmt.Sprintf("%s (%v)", app.Name, number_per_name[app.Name])] = app
				app.Number = number_per_name[app.Name]
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

func get_dirs(
	home string,
) []string {
	var dirs []string
	data_dirs := os.Getenv("XDG_DATA_DIRS")
	data_home := os.Getenv("XDG_DATA_HOME")
	if data_dirs == "" {
		data_dirs = "/usr/share/:/usr/local/share/"
	}
	if data_home == "" {
		data_home = filepath.Join(home, ".local/share/")
	}
	dirs = append(dirs, filepath.Join(data_home, "applications/"))
	for dir := range strings.SplitSeq(data_dirs, ":") {
		dirs = append(dirs, filepath.Join(dir, "applications/"))
	}

	return dirs
}
