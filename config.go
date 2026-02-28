package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

func print_error(
	err error,
) {
	fmt.Printf("Failed to load config: %s\n", err.Error())
}

func load_config(
	path string,
	wg *sync.WaitGroup,
	config_chan chan<- Config,
) {
	defer wg.Done()
	var config Config

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
			config_chan <- default_config
			print_error(err)
		}

		json_string, err := json.MarshalIndent(default_config, "", "    ")
		if err != nil {
			config_chan <- default_config
			print_error(err)
		}

		_, err = file.WriteString(string(json_string))
		if err != nil {
			config_chan <- default_config
			print_error(err)
		}

		config = default_config
	} else {
		byte_data, err := io.ReadAll(file)
		if err != nil {
			config_chan <- default_config
			print_error(err)
		}
		defer file.Close()

		if err = json.Unmarshal(byte_data, &config); err != nil {
			config = default_config
			print_error(err)
		}
	}

	config_chan <- config
}
