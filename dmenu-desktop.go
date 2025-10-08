package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
)

func main() {
	var dirs []string
	args := os.Args
	home := os.Getenv("HOME")
	lang := regexp.MustCompile("_.*").ReplaceAllString(os.Getenv("LANG"), "")
	data_dirs := os.Getenv("XDG_DATA_DIRS")
	data_home := os.Getenv("XDG_DATA_HOME")
	config_home := os.Getenv("XDG_CONFIG_HOME")
	if config_home == "" {
		config_home = filepath.Join(home, ".config/")
	}
	config_path := filepath.Join(config_home, "dmenu-desktop-go/config.json")
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

	var wg sync.WaitGroup
	var config_wg sync.WaitGroup
	files_chan := make(chan []string, len(dirs))
	apps_chan := make(chan App)
	config_chan := make(chan Config, 1)

	config_wg.Add(1)
	go load_config(config_path, &config_wg, config_chan)

	for _, dir := range dirs {
		wg.Add(1)
		go find_files_with_extension(dir, ".desktop", &wg, files_chan)
	}

	go func() {
		wg.Wait()
		close(files_chan)
	}()

	var files []string
	for values := range files_chan {
		files = append(files, values...)
	}

	for _, file := range files {
		wg.Add(1)
		go get_desktop_details(
			file,
			lang,
			regexp.MustCompile(fmt.Sprintf("(^%s/)", strings.Join(dirs, "/|^"))),
			&wg,
			apps_chan,
		)
	}

	go func() {
		wg.Wait()
		close(apps_chan)
	}()

	var apps []App
	for app := range apps_chan {
		apps = append(apps, app)
	}

	apps_final := remove_duplicates(apps, dirs)

	go func() {
		config_wg.Wait()
		close(config_chan)
	}()
	config := <-config_chan

	var names []string
	for name := range apps_final {
		if !slices.Contains(config.Excludes, name) {
			names = append(names, name)
		}
	}
	for name := range config.Aliases {
		if !slices.Contains(append(names, config.Excludes...), name) {
			names = append(names, name)
		}
	}

	sort.Strings(names)
	var stdin bytes.Buffer
	for _, name := range names {
		stdin.WriteString(name + "\n")
	}

	fmt.Printf("Read %v .desktop files, found %v apps\n", len(files), len(names))

	command_args, err := parse_command(config.MenuCommand)
	if err != nil {
		log.Fatalf("Failed to parse menu command: %s", err.Error())
	}
	command_args = append(command_args, args[1:]...)
	cmd := exec.Command(command_args[0], command_args[1:]...)
	cmd.Stdin = bytes.NewReader(stdin.Bytes())
	output, err := cmd.Output()
	if err != nil {
		log.Fatalf("Menu failed: %s", err.Error())
	}
	selected := strings.TrimSpace(string(output))

	err = run(selected, config, apps_final)
	if err != nil {
		log.Fatalf("Selected command failed: %s", err.Error())
	}
}
