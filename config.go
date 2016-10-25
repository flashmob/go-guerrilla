package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type GlobalConfig struct {
	Allowed_hosts        string         `json:"allowed_hosts"`
	Primary_host         string         `json:"primary_mail_host"`
	Verbose              bool           `json:"verbose"`
	Mysql_table          string         `json:"mail_table"`
	Mysql_db             string         `json:"mysql_db"`
	Mysql_host           string         `json:"mysql_host"`
	Mysql_pass           string         `json:"mysql_pass"`
	Mysql_user           string         `json:"mysql_user"`
	Servers              []ServerConfig `json:"servers"`
	Pid_file             string         `json:"pid_file,omitempty"`
	Save_workers_size    int            `json:"save_workers_size"`
	Redis_expire_seconds int            `json:"redis_expire_seconds"`
	Redis_interface      string         `json:"redis_interface"`
}

type ServerConfig struct {
	Is_enabled       bool   `json:"is_enabled"`
	Host_name        string `json:"host_name"`
	Max_size         int    `json:"max_size"`
	Private_key_file string `json:"private_key_file"`
	Public_key_file  string `json:"public_key_file"`
	Timeout          int    `json:"timeout"`
	Listen_interface string `json:"listen_interface"`
	Start_tls_on     bool   `json:"start_tls_on,omitempty"`
	Tls_always_on    bool   `json:"tls_always_on,omitempty"`
	Max_clients      int    `json:"max_clients"`
	Log_file         string `json:"log_file"`
}

var mainConfig GlobalConfig
var flagVerbose, flagIface, flagConfigFile string

// config is read at startup, or when a SIG_HUP is caught
func readConfig() {
	log.SetOutput(os.Stdout)
	// parse command line arguments
	if !flag.Parsed() {
		flag.StringVar(&flagConfigFile, "config", "goguerrilla.conf", "Path to the configuration file")
		flag.StringVar(&flagVerbose, "v", "n", "Verbose, [y | n] ")
		flag.StringVar(&flagIface, "if", "", "Interface and port to listen on, eg. 127.0.0.1:2525 ")
		flag.Parse()
	}
	// load in the config.
	b, err := ioutil.ReadFile(flagConfigFile)
	if err != nil {
		log.Fatalln("Could not read config file", err)
	}

	mainConfig = GlobalConfig{}
	err = json.Unmarshal(b, &mainConfig)
	//fmt.Println(theConfig)
	//fmt.Println(fmt.Sprintf("allowed hosts: %s", theConfig.Allowed_hosts))
	//log.Fatalln("Could not parse config file:", theConfig)
	if err != nil {
		fmt.Println("Could not parse config file:", err)
		log.Fatalln("Could not parse config file:", err)
	}

	// copy command line flag over so it takes precedence
	if len(flagVerbose) > 0 && strings.ToUpper(flagVerbose) == "Y" {
		mainConfig.Verbose = true
	}

	if len(flagIface) > 0 {
		mainConfig.Servers[0].Listen_interface = flagIface
	}
	// map the allow hosts for easy lookup
	if len(mainConfig.Allowed_hosts) > 0 {
		if arr := strings.Split(mainConfig.Allowed_hosts, ","); len(arr) > 0 {
			for i := 0; i < len(arr); i++ {
				allowedHosts[arr[i]] = true
			}
		}
	} else {
		log.Fatalln("Config error, GM_ALLOWED_HOSTS must be s string.")
	}
	if mainConfig.Pid_file == "" {
		mainConfig.Pid_file = "/var/run/go-guerrilla.pid"
	}

	return
}
