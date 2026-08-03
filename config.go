package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

func saveConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePosition, data, 0644)
}

func getDefaultConfig() Config {
	return Config{
		TCP_HOST:      "0.0.0.0:6969",
		RCON_HOST:     "0.0.0.0:25575",
		RCON_PASSWORD: "1234abcd",
		PORT:          "8080",
		START_COMMAND: "java -Xmx6G -jar ./server.jar nogui",
		SERVER_PATH:   "$HOME/server/",
		SHOW_LOG:      true,
		FILES: []FileConfig{
			{
				PATH:    "$HOME/server/server.properties",
				NAME:    "server.properties",
				PREVIEW: true,
			},
		},
	}
}
func CheckExist(name string) (bool, error) {
	_, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, fmt.Errorf("failed to get file status: %s", name)
		}
	}
	return true, nil
}

func loadConfig(file string) {
	b, err := CheckExist(file)
	fmt.Println("checking " + file + " file")
	if err != nil {
		log.Fatalf("Error occoured when checking config.json file, make sure that is exists: %v", err)
		os.Exit(1)
	}
	if !b {
		log.Println("config.json does not exit, creating...")
		webConfig = getDefaultConfig()
		err := saveConfig(webConfig)
		if err != nil {
			log.Println("failed to save config.json, please download it from the repo")
		}
	} else {
		fileContent, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("Error occoured when reading: %v", err)
			os.Exit(1)
		}
		if err := json.Unmarshal(fileContent, &webConfig); err != nil {
			log.Fatalf("Error unamarshalling JSON: %v", err)
		}
		log.Println("Config loaded successfully")
		log.Println("====================================")
		log.Printf("rcon_host: %s\n", webConfig.RCON_HOST)
		log.Printf("rcon_password: %s\n", webConfig.RCON_PASSWORD)
		log.Printf("port: %s\n", webConfig.PORT)
		log.Printf("server_path: %s\n", webConfig.SERVER_PATH)
		log.Printf("start_command: %s\n", webConfig.START_COMMAND)
		log.Printf("show_log: %t\n", webConfig.SHOW_LOG)
		log.Printf("debug: %t\n", DEBUG)
		log.Println("====================================")
	}
	webConfig.SERVER_PATH = os.ExpandEnv(webConfig.SERVER_PATH)
	for _, file := range webConfig.FILES {
		file.PATH = os.ExpandEnv(file.PATH)
	}
	for _, f := range webConfig.FILES {
		editFile := f.newFile()
		editConfigFiles.RegisterHandler(editFile.ID, *editFile)
	}
	log.Println("edit config files loaded successfully")
}
