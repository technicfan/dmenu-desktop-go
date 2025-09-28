package main

import (
	"bytes"
	"encoding/json"
	"errors"

	"fmt"
	"io"
	"syscall"

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

const (
	reserved_chars = " '\\><~|&;$*?#()`\"\t\n"
)

type App struct {
	Name string `json:"name"`
	File string `json:"file"`
	Id   string `json:"id"`
}

type Alias struct {
	Command   string `json:"command"`
	IsDesktop bool   `json:"is_desktop"`
}

type Config struct {
	MenuCommand     string           `json:"menu_command"`
	TerminalCommand string           `json:"terminal_command"`
	CacheEnabled    bool             `json:"cache_enabled"`
	Aliases         map[string]Alias `json:"aliases"`
	Excludes        []string         `json:"excludes"`
}

func load_config(path string) (Config, error) {
	default_config := Config{
		"dmenu -i -p Run:",
		"kitty",
		true,
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

	return data, info.ModTime().Unix(), nil
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

func clean_cache(apps []App, cache_time int64) []App {
	var clean_apps []App
	for _, app := range apps {
		if info, err := os.Stat(app.File); err == nil && info.ModTime().Unix() < cache_time {
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

	re := regexp.MustCompile(`(?s)(?m)^\[Desktop Entry\](.*?)(^\[|\z)`)
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
			strings.Replace(matches[0],
				fmt.Sprintf("Name[%s]=", lang), "", 1),
			path,
			id,
		}
	} else {
		re = regexp.MustCompile("(?m)^Name=.*")
		matches = re.FindStringSubmatch(desktop_entry)
		if len(matches) > 0 {
			apps <- App{strings.Replace(matches[0], "Name=", "", 1), path, id}
		}
	}
}

func find_files(path string, cache_time int64, wg *sync.WaitGroup, files_chan chan<- []string) {
	defer wg.Done()

	var files []string

	filepath.WalkDir(path, func(file string, dir fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if info, err := dir.Info(); err == nil {
			var creation_time int64
			mod_time := info.ModTime().Unix()

			switch stats := info.Sys().(type) {
			case *syscall.Stat_t:
				creation_time = stats.Ctim.Sec
			case *unix.Stat_t:
				creation_time = stats.Ctim.Sec
			default:
				creation_time = mod_time
			}

			if (info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) &&
				strings.EqualFold(filepath.Ext(dir.Name()), ".desktop") &&
				(mod_time > cache_time || creation_time > cache_time) {
				files = append(files, file)
			}
		}

		return nil
	})

	files_chan <- files
}

func split_args(command string) ([]string, error) {
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

	return splits, nil
}

func run_desktop(path string, config Config) error {
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
		command, err = split_args(command_string)
		if err != nil {
			return err
		}
	}

	re = regexp.MustCompile("(?m)^Path=.*")
	matches = re.FindStringSubmatch(desktop_entry)
	if len(matches) != 0 {
		log.Print(matches[0])
		os.Chdir(strings.Replace(matches[0], "Path=", "", 1))
	}

	err = Exec(command)

	return err
}

func Exec(command []string) error {
	binary, err := exec.LookPath(command[0])
	if err != nil {
		return err
	}

	command[0] = binary

	err = unix.Exec(binary, command, os.Environ())

	return err
}

func run(name string, config Config, apps map[string]App) error {
	if alias, exists := config.Aliases[name]; exists {
		if !alias.IsDesktop {
			command, err := split_args(alias.Command)
			if err != nil {
				return err
			}
			err = Exec(command)

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

	command, err := split_args(name)
	if err != nil {
		return err
	}
	err = Exec(command)

	return err
}

func main() {
	usr, _ := user.Current()
	home := usr.HomeDir
	args := os.Args
	re := regexp.MustCompile("_.*")

	var dirs []string
	config_path := filepath.Join(home, ".config/dmenu-desktop-go/config.json")
	cache_path := filepath.Join(home, ".cache/dmenu-desktop-go.json")
	lang := re.ReplaceAllString(os.Getenv("LANG"), "")
	data_dirs := os.Getenv("XDG_DATA_DIRS")
	data_home := os.Getenv("XDG_DATA_HOME")
	if data_dirs == "" {
		data_dirs = "/usr/share/:/usr/local/share/"
	}
	if data_home == "" {
		data_home = ".local/share/"
	}
	for dir := range strings.SplitSeq(data_dirs, ":") {
		dirs = append(dirs, filepath.Join(dir, "applications/"))
	}
	dirs = append(dirs, filepath.Join(home, data_home, "applications/"))

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
	} else if config.CacheEnabled {
		cached_apps, time, err = read_cache(cache_path)
		if err != nil {
			log.Print("Failed to load cache")
		}
		cached_apps = clean_cache(cached_apps, time)
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
	for app := range apps_chan {
		apps = append(apps, app)
	}
	for _, app := range cached_apps {
		apps = append(apps, app)
	}

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

	if config.CacheEnabled {
		err = write_cache(cache_path, apps)
		if err != nil {
			fmt.Printf("Failed to write cache: %s", err.Error())
		}
	}

	sort.Strings(names)
	var stdin bytes.Buffer
	for _, name := range names {
		stdin.WriteString(name + "\n")
	}

	command_args, err := split_args(config.MenuCommand)
	if err != nil {
		log.Fatal(err)
	}
	command_args = append(command_args, args[1:]...)
	if len(command_args) == 0 {
		log.Fatal("Menu command cannot be empty")
	} else {
		cmd := exec.Command(command_args[0], command_args[1:]...)
		cmd.Stdin = bytes.NewReader(stdin.Bytes())
		output, err := cmd.Output()
		if err != nil {
			log.Fatal(err)
		}
		selected := strings.TrimSpace(string(output))

		err = run(selected, config, apps_by_name)
		if err != nil {
			log.Fatal(err)
		}
	}
}
