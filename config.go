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
	Allowed_hosts        string         `json:"GM_ALLOWED_HOSTS"`
	Primary_host         string         `json:"GM_PRIMARY_MAIL_HOST"`
	Verbose              bool           `json:"verbose"`
	Mysql_table          string         `json:"GM_MAIL_TABLE"`
	Mysql_db             string         `json:"MYSQL_DB"`
	Mysql_host           string         `json:"MYSQL_HOST"`
	Mysql_pass           string         `json:"MYSQL_PASS"`
	Mysql_user           string         `json:"MYSQL_USER"`
	Servers              []ServerConfig `json:"servers"`
	Pid_file             string         `json:"pid_file,omitempty"`
	Save_workers_size    int            `json:"save_workers_size"`
	redis_expire_seconds int            `json:"redis_expire_seconds"`
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
	Is_tls_on        bool   `json:"is_tls_on,omitempty"`
	Max_clients      int    `json:"max_clients"`
	Log_file         string `json:"log_file"`
}

// defaults. Overwrite any of these in the configure() function which loads them from a json file
/*
var gConfig = map[string]interface{}{
	"GSMTP_MAX_SIZE":            "131072",
	"GSMTP_HOST_NAME":           "server.example.com", // This should also be set to reflect your RDNS
	"GSMTP_VERBOSE":             "Y",
	"GSMTP_LOG_FILE":            "",    // Eg. /var/log/goguerrilla.log or leave blank if no logging
	"GSMTP_TIMEOUT":             "100", // how many seconds before timeout.
	"MYSQL_HOST":                "127.0.0.1:3306",
	"MYSQL_USER":                "gmail_mail",
	"MYSQL_PASS":                "ok",
	"MYSQL_DB":                  "gmail_mail",
	"GM_MAIL_TABLE":             "new_mail",
	"GSTMP_LISTEN_INTERFACE":    "0.0.0.0:25",
	"GSMTP_PUB_KEY":             "/etc/ssl/certs/ssl-cert-snakeoil.pem",
	"GSMTP_PRV_KEY":             "/etc/ssl/private/ssl-cert-snakeoil.key",
	"GM_ALLOWED_HOSTS":          "guerrillamail.de,guerrillamailblock.com",
	"GM_PRIMARY_MAIL_HOST":      "guerrillamail.com",
	"GM_MAX_CLIENTS":            "500",
	"NGINX_AUTH_ENABLED":        "N",              // Y or N
	"NGINX_AUTH":                "127.0.0.1:8025", // If using Nginx proxy, ip and port to serve Auth requsts
	"PID_FILE":                  "/var/run/go-guerrilla.pid",
	"GSMTP_MAIL_EXPIRE_SECONDS": "72000",
}

*/
var theConfig GlobalConfig
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

	theConfig = GlobalConfig{}
	err = json.Unmarshal(b, &theConfig)
	//fmt.Println(theConfig)
	//fmt.Println(fmt.Sprintf("allowed hosts: %s", theConfig.Allowed_hosts))
	//log.Fatalln("Could not parse config file:", theConfig)
	if err != nil {
		fmt.Println("Could not parse config file:", err)
		log.Fatalln("Could not parse config file:", err)
	}

	// copy command line flag over so it takes precedence
	if len(flagVerbose) > 0 && strings.ToUpper(flagVerbose) == "Y" {
		theConfig.Verbose = true
	}

	if len(flagIface) > 0 {
		theConfig.Servers[0].Listen_interface = flagIface
	}
	// map the allow hosts for easy lookup
	if len(theConfig.Allowed_hosts) > 0 {
		if arr := strings.Split(theConfig.Allowed_hosts, ","); len(arr) > 0 {
			for i := 0; i < len(arr); i++ {
				allowedHosts[arr[i]] = true
			}
		}
	} else {
		log.Fatalln("Config error, GM_ALLOWED_HOSTS must be s string.")
	}
	if theConfig.Pid_file == "" {
		theConfig.Pid_file = "/var/run/go-guerrilla.pid"
	}

	return
}
