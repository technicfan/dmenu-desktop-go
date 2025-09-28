package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
)

func print_error(err error) {
	log.Printf("Failed to load config: %s", err.Error())
}

func load_config(
	path string,
	wg *sync.WaitGroup,
	config_chan chan<- Config,
) {
	defer wg.Done()

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

		config_chan <- default_config
	}

	byte_data, err := io.ReadAll(file)
	if err != nil {
		config_chan <- default_config
		print_error(err)
	}
	defer file.Close()

	var config Config
	if err = json.Unmarshal(byte_data, &config); err != nil {
		config_chan <- default_config
		print_error(err)
	}

	config_chan <- config
}
