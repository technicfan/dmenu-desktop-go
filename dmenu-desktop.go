package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

func main() {
	args := os.Args
	home := os.Getenv("HOME")
	localized_name_key := fmt.Sprintf("Name[%s]=", strings.Split(os.Getenv("LANG"), "_")[0])
	config_home := os.Getenv("XDG_CONFIG_HOME")
	if config_home == "" {
		config_home = filepath.Join(home, ".config/")
	}
	config_path := filepath.Join(config_home, "dmenu-desktop-go/config.json")
	dirs := get_dirs(home)

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

	go func() {
		config_wg.Wait()
		close(config_chan)
	}()
	config := <-config_chan

	for _, file := range files {
		wg.Add(1)
		go get_app_async(
			file,
			localized_name_key,
			config.TerminalCommand,
			dirs,
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
		fmt.Printf("Failed to parse menu command: %s", err.Error())
		os.Exit(1)
	}
	command_args = append(command_args, args[1:]...)
	cmd := exec.Command(command_args[0], command_args[1:]...)
	cmd.Stdin = bytes.NewReader(stdin.Bytes())
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Menu failed: %s", err.Error())
		os.Exit(err.(*exec.ExitError).ExitCode())
	}
	selected := strings.TrimSpace(string(output))

	err = run(selected, config, apps_final, localized_name_key, dirs)
	if err != nil {
		fmt.Printf("Selected command failed: %s", err.Error())
		os.Exit(1)
	}
}
