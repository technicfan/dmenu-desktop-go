package main

import (
	"fmt"
	"io"
	"io/fs"
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

func find_desktop_files(
	path string,
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
				strings.HasSuffix(dir_entry.Name(), ".desktop") {
				files = append(files, file)
			}
		}

		return nil
	})

	files_chan <- files
}
