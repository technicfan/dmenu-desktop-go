package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

type App struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Id     string `json:"id"`
}

type Alias struct {
	Command   string `json:"command"`
	IsDesktop bool   `json:"is_desktop"`
}

type Config struct {
	MenuCommand     string           `json:"menu_command"`
	TerminalCommand string           `json:"terminal_command"`
	Aliases         map[string]Alias `json:"aliases"`
	Excludes        []string         `json:"excludes"`
}

func load_config(path string) (Config, error) {
	default_config := Config{
		"dmenu -i -p Run:",
		"kitty",
		map[string]Alias{},
		[]string{},
	}
	file, err := os.Open(path)
	if err != nil {
		file, err := os.Create(path)
		if err != nil {
			return default_config, err
		}

		json_string, err := json.MarshalIndent(default_config, "", "    ")
		if err != nil {
			return default_config, err
		}

		_, err = file.WriteString(string(json_string))
		if err != nil {
			return default_config, err
		}

		return default_config, nil
	}

	byte_data, err := io.ReadAll(file)
	if err != nil {
		return default_config, err
	}
	defer file.Close()

	var config Config
	if err = json.Unmarshal(byte_data, &config); err != nil {
		return default_config, err
	}

	return config, nil
}

func read_cache(path string) ([]App, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}

	byte_data, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	var data []App
	err = json.Unmarshal(byte_data, &data)
	if err != nil {
		return nil, 0, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}

	return clean_cache(data), info.ModTime().Unix(), nil
}

func write_cache(path string, data []App) error {
	var file *os.File
	file, err := os.OpenFile(path, os.O_TRUNC|os.O_RDWR, os.ModePerm)
	if err != nil {
		file, err = os.Create(path)
		if err != nil {
			return err
		}
	}
	defer file.Close()

	json_string, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}

	_, err = file.WriteString(string(json_string))
	if err != nil {
		return err
	}

	return nil
}

func clean_cache(apps []App) []App {
	var clean_apps []App
	for _, app := range apps {
		if _, err := os.Stat(app.File); err == nil {
			clean_apps = append(clean_apps, app)
		}
	}

	return clean_apps
}

func get_desktop_string(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	byte_data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`(?s)^\[Desktop Entry\](.*?)(^\[|\z)`)
	matches := re.FindStringSubmatch(string(byte_data))
	if len(matches) == 0 {
		return "", nil
	}

	return matches[0], nil
}

func get_details(path string, lang string, wg *sync.WaitGroup, apps chan<- App) {
	defer wg.Done()

	desktop_entry, err := get_desktop_string(path)
	if err != nil {
		log.Fatal(err)
	}
	if desktop_entry == "" {
		return
	}

	if strings.Contains(desktop_entry, "Hidden=true") ||
		strings.Contains(desktop_entry, "NoDisplay=true") ||
		!strings.Contains(desktop_entry, "Type=Application") {
		return
	}

	id := strings.ReplaceAll(filepath.Base(path), ".desktop", "")

	re := regexp.MustCompile(fmt.Sprintf(`Name\[%s\]=.*`, lang))
	matches := re.FindStringSubmatch(desktop_entry)
	if len(matches) > 0 {
		apps <- App{
			strings.Replace(matches[0],
			fmt.Sprintf("Name[%s]=", lang), "", 1),
			path,
			id,
		}
	} else {
		re = regexp.MustCompile("Name=.*")
		matches = re.FindStringSubmatch(desktop_entry)
		if len(matches) > 0 {
			apps <- App{strings.Replace(matches[0], "Name=", "", 1), path, id}
		}
	}
}

func find_files(path string, time int64, wg *sync.WaitGroup, files_chan chan<- []string) {
	defer wg.Done()

	info, err := os.Stat(path)
	if err != nil {
		log.Fatal(err)
	}
	if time > info.ModTime().Unix() {
		return
	}

	var files []string

	filepath.WalkDir(path, func(file string, dir fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if info, err := dir.Info(); err == nil {
			if (info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) &&
				strings.EqualFold(filepath.Ext(dir.Name()), ".desktop") &&
				info.ModTime().Unix() > time {
				files = append(files, file)
			}
		}

		return nil
	})

	files_chan <- files
}

func run_desktop(path string, config Config) error {
	desktop_entry, err := get_desktop_string(path)
	if err != nil {
		return err
	}
	if desktop_entry == "" {
		return fmt.Errorf("%s is invalid", path)
	}

	re := regexp.MustCompile("Exec=.*")
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
		command = strings.Split(command_string, " ")
	}

	re = regexp.MustCompile("^Path=.*")
	matches = re.FindStringSubmatch(desktop_entry)
	if len(matches) != 0 {
		os.Chdir(strings.Replace(matches[0], "Path=", "", 1))
	}

	err = Exec(command)

	return err
}

func Exec(command []string) error {
	for i, value := range command {
		if strings.Contains(value, "=") {
			command = slices.Delete(command, i, i + 1)
			variable := strings.Split(value, "=")
			os.Setenv(variable[0], variable[1])
		}
	}

	cmd := exec.Command("which", command[0])
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	binary := strings.TrimSpace(string(output))
	command[0] = binary

	err = unix.Exec(binary, command, os.Environ())

	return err
}

func run(name string, config Config, apps map[string]App) error {
	if alias, exists := config.Aliases[name]; exists {
		if !alias.IsDesktop {
			command := strings.Split(alias.Command, " ")
			err := Exec(command)

			return err
		} else if app, exists := apps[alias.Command]; exists {
			err := run_desktop(app.File, config)

			return err
		}
	}

	if app, exists := apps[name]; exists {
		err := run_desktop(app.File, config)

		return err
	}

	command := strings.Split(name, " ")
	err := Exec(command)

	return err
}

func main() {
	usr, _ := user.Current()
	home := usr.HomeDir
	args := os.Args
	re := regexp.MustCompile("_.*")

	// config_path := filepath.Join(home, ".config_path/dmenu-desktop-go/config_path.json")
	// cache := filepath.Join(home, ".cache/dmenu-desktop-go.json")
	config_path := "./config.json"
	cache_path := "./cache.json"
	lang := re.ReplaceAllString(os.Getenv("LANG"), "")
	dirs := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		filepath.Join(home, ".local/share/applications"),
		"/var/lib/flatpak/exports/share/applications",
		filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
	}

	var wg sync.WaitGroup
	files_chan := make(chan []string, len(dirs))
	apps_chan := make(chan App)

	config, err := load_config(config_path)
	if err != nil {
		log.Print(err)
	}

	var cached_apps []App
	time := int64(0)

	if slices.Contains(args, "--clean") {
		index := slices.Index(args, "--clean")
		args = slices.Delete(args, index, index + 1)
	} else {
		cached_apps, time, err = read_cache(cache_path)
		if err != nil {
			log.Print("Failed to load cache")
		}
	}

	for _, dir := range dirs {
		wg.Add(1)
		go find_files(dir, time, &wg, files_chan)
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
		go get_details(file, lang, &wg, apps_chan)
	}

	go func() {
		wg.Wait()
		close(apps_chan)
	}()

	apps_by_name := make(map[string]App)

	var apps []App
	for _, app := range cached_apps {
		apps = append(apps, app)
	}
	for app := range apps_chan {
		apps = append(apps, app)
	}

	apps_by_id := make(map[string]App)
	for _, app := range apps {
		add := true
		if found, exists := apps_by_id[app.Id]; exists {
			if app.File > found.File {
				add = false
			} else {
				delete(apps_by_name, found.Name)
			}
		}
		if add {
			apps_by_name[app.Name] = app
			apps_by_id[app.Id] = app
		}
	}

	var names []string
	for name := range apps_by_name {
		if !slices.Contains(config.Excludes, name) {
			names = append(names, name)
		}
	}
	for name := range config.Aliases {
		if !slices.Contains(config.Excludes, name) {
			names = append(names, name)
		}
	}

	err = write_cache(cache_path, apps)
	if err != nil {
		fmt.Printf("Failed to write cache: %s", err.Error())
	}

	sort.Strings(names)
	var stdin bytes.Buffer
	for _, name := range names {
		stdin.WriteString(name + "\n")
	}

	command_args := strings.Split(config.MenuCommand, " ")
	cmd := exec.Command(command_args[0], command_args[1:]...)
	cmd.Stdin = bytes.NewReader(stdin.Bytes())
	output, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	selected := strings.TrimSpace(string(output))
	fmt.Printf("Selected \"%s\"\n", selected)

	err = run(selected, config, apps_by_name)
	if err != nil {
		log.Fatal(err)
	}
}
