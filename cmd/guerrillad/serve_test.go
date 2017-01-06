package main

import (
	"github.com/flashmob/go-guerrilla"
	"testing"
)

var configJsonA = `
{
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_name": "dummy",
    "backend_config": {
        "log_received_mails": true
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "private_key_file":"/path/to/pem/file/test.com.key",
            "public_key_file":"/path/to/pem/file/test.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:25",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        }
    ]
}
`

var configJsonB = `
{
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_name": "dummy",
    "backend_config": {
        "log_received_mails": false
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "private_key_file":"/path/to/pem/file/test.com.key",
            "public_key_file":"/path/to/pem/file/test.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:25",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        }
    ]
}
`
var configJsonC = `
{
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_name": "guerrilla-redis-db",
    "backend_config" :
        {
            "mysql_db":"gmail_mail",
            "mysql_host":"127.0.0.1:3306",
            "mysql_pass":"ok",
            "mysql_user":"root",
            "mail_table":"new_mail",
            "redis_interface" : "127.0.0.1:6379",
            "redis_expire_seconds" : 7200,
            "save_workers_size" : 3,
            "primary_mail_host":"sharklasers.com"
        },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "private_key_file":"/path/to/pem/file/test.com.key",
            "public_key_file":"/path/to/pem/file/test.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:25",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        },
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size":1000000,
            "private_key_file":"/path/to/pem/file/test.com.key",
            "public_key_file":"/path/to/pem/file/test.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:465",
            "start_tls_on":false,
            "tls_always_on":true,
            "max_clients":500
        }
    ]
}
`

// make sure that we get all the config change events
func TestCmdConfigChangeEvents(t *testing.T) {
	oldconf := &CmdConfig{}
	oldconf.load([]byte(configJsonA))

	newconf := &CmdConfig{}
	newconf.load([]byte(configJsonB))

	newerconf := &CmdConfig{}
	newerconf.load([]byte(configJsonC))

	expectedEvents := map[string]bool{
		"config_change:backend_config": false,
		"config_change:backend_name":   false,
	}
	toUnsubscribe := map[string]func(c *CmdConfig){}

	for event := range expectedEvents {
		// Put in anon func since range is overwriting event
		func(e string) {
			f := func(c *CmdConfig) {
				expectedEvents[e] = true
			}
			guerrilla.Bus.Subscribe(event, f)
			toUnsubscribe[event] = f

		}(event)
	}

	// emit events
	newconf.emitChangeEvents(oldconf)
	newerconf.emitChangeEvents(newconf)
	// unsubscribe
	for unevent, unfun := range toUnsubscribe {
		guerrilla.Bus.Unsubscribe(unevent, unfun)
	}

	for event, val := range expectedEvents {
		if val == false {
			t.Error("Did not fire config change event:", event)
			t.FailNow()
			break
		}
	}
}
