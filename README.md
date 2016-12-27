
Go-Guerrilla SMTPd
====================

An minimalist SMTP server written in Go, made for receiving large volumes of mail.

![Go Guerrilla](/GoGuerrilla.png)

### What is Go Guerrilla SMTPd?

It's a small SMTP server written in Go, for the purpose of receiving large volume of email.
Written for GuerrillaMail.com which processes hundreds of thousands of emails
every hour.

The purpose of this daemon is to grab the email, save it to the database
and disconnect as quickly as possible.

A typical user of this software would probably want to look into 
`backends/guerrilla_db_redis.go` source file to use as an example to 
customize for their own systems.

This server does not attempt to filter HTML, check for spam or do any 
sender verification. These steps should be performed by other programs,
 (or perhaps your own custom backend?).
The server does not send any email including bounces.

The software is using MIT License (MIT) - contributors welcome.

### Roadmap / Contributing & Bounties


Pull requests / issue reporting & discussion / code reviews always 
welcome. To encourage more pull requests, we are now offering bounties 
funded from our bitcoin donation address:

`1grr11aWtbsyMUeB4EGfHvTuu7eFzkJ4A`

So far we have the following bounties are still open:
(Updated 22 Dec 2016)

- Let's encrypt TLS certificate support! 
Take a look at https://github.com/flashmob/go-guerrilla/issues/29
(0.5 for a successful merge)

- Analytics: A web based admin panel that displays live statistics,
including the number of clients, memory usage, graph the number of
connections/bytes/memory used for the last 24h.
Show the top source clients by: IP, by domain & by HELO message.
Using websocket via https & password protected.
(1 BTC for a successful merge)

- Testing: Automated test that can start the server and test end-to-end
a few common cases, some unit tests would be good too. Automate to
 run when code is pushed to github
(0.25 BTC for a successful merge)

- Profiling: Simulate a configurable number of simultaneous clients 
(eg 5000) which send commands at random speeds with messages of various 
lengths. Some connections to use TLS. Some connections may produce 
errors, eg. disconnect randomly after a few commands, issue unexpected
input or timeout. Provide a report of all the bottlenecks and setup so 
that the report can be run automatically run when code is pushed to 
github.
(0.25 BTC)

- Looking for someone to do a code review & possibly fix any tidbits,
they find, or suggestions for doing things better.
(Already one bounty of 0.25 paid, however, more is always welcome)

Ready to roll up your sleeves and have a go?
Please open an issue for more clarification / details on Github.
Also, welcome your suggestions for adding things to this Roadmap - please open an issue.

Another way to contribute is to donate to our bitcoin address to help
us fund more bounties!
`1grr11aWtbsyMUeB4EGfHvTuu7eFzkJ4A`

### Brief History and purpose

Go-Guerrilla is used as the primary server for receiving email at
Guerrilla Mail. As of 2016, it's handling all connections without any
proxy (Nginx).

Originally, Guerrilla Mail ran Exim which piped email to a php script (2009).
As as the site got popular and more email came through, this approach
eventually swamped the server.

The next solution was to decrease the heavy setup into something more
lightweight. A small script was written to implement a basic SMTP server (2010).
Eventually that script also got swamped, so it was re-written to use
event driven I/O (2012). A year later, the latest script also became inadequate
 so it was ported to Go and has served us well since.


Getting started
===========================

(Assuming that you have GNU make and latest Go on your system)

To build, just run

```
$ make guerrillad
```

Rename goguerrilla.conf.sample to goguerrilla.conf

See `backends/guerrilla_db_redis.go` source to use an example for creating your own email saving backend, 
or the dummy one if you'd like to start from scratch.

If you want to build on the sample `guerrilla-db-redis` module, setup the following table
in MySQL:

	CREATE TABLE IF NOT EXISTS `new_mail` (
	  `mail_id` int(11) NOT NULL auto_increment,
	  `date` datetime NOT NULL,
	  `from` varchar(128) character set latin1 NOT NULL,
	  `to` varchar(128) character set latin1 NOT NULL,
	  `subject` varchar(255) NOT NULL,
	  `body` text NOT NULL,
	  `charset` varchar(32) character set latin1 NOT NULL,
	  `mail` longblob NOT NULL,
	  `spam_score` float NOT NULL,
	  `hash` char(32) character set latin1 NOT NULL,
	  `content_type` varchar(64) character set latin1 NOT NULL,
	  `recipient` varchar(128) character set latin1 NOT NULL,
	  `has_attach` int(11) NOT NULL,
	  `ip_addr` varchar(15) NOT NULL,
	  `delivered` bit(1) NOT NULL default b'0',
	  `attach_info` text NOT NULL,
	  `dkim_valid` tinyint(4) default NULL,
	  PRIMARY KEY  (`mail_id`),
	  KEY `to` (`to`),
	  KEY `hash` (`hash`),
	  KEY `date` (`date`)
	) ENGINE=InnoDB  DEFAULT CHARSET=utf8

The above table does not store the body of the email which makes it quick
to query and join, while the body of the email is fetched from Redis
if needed.

You can implement your own saveMail function to use whatever storage /
backend fits for you. Please share them ^_^, in particular, we would 
like to see other formats such as maildir and mbox.


Use as a package
============================
Guerrilla SMTPd can also be imported and used as a package in your project.

## Import Guerrilla.
```go
import "github.com/flashmob/go-guerrilla"
```

## Implement the `Backend` interface
Or use one of the implementations in the `backends` sub-package). This is how
your application processes emails received by the Guerrilla app.
```go
type CustomBackend struct {...}

func (cb *CustomBackend) Process(c *guerrilla.Envelope) guerrilla.BackendResult {
  err := saveSomewhere(c.Data)
  if err != nil {
    return guerrilla.NewBackendResult(fmt.Sprintf("554 Error: %s", err.Error()))
  }
  return guerrilla.NewBackendResult("250 OK")
}
```

## Create an app instance.
See Configuration section below for setting configuration options.
```go
config := &guerrilla.AppConfig{
  Servers: []*guerrilla.ServerConfig{...},
  AllowedHosts: []string{...}
}
backend := &CustomBackend{...}
app := guerrilla.New(config, backend)
```

## Start the app.
`Start` is non-blocking, so make sure the main goroutine is kept busy
```go
app.Start()
```


Configuration
============================================
The configuration is in strict JSON format. Here is an annotated configuration.
Copy goguerrilla.conf.sample to goguerrilla.conf


    {
        "allowed_hosts": ["guerrillamail.com","guerrillamailblock.com","sharklasers.com","guerrillamail.net","guerrillamail.org"], // What hosts to accept
        "pid_file" : "/var/run/go-guerrilla.pid", // pid = process id, so that other programs can send signals to our server
                "backend_name": "guerrilla-db-redis", // what backend to use for saving email. See /backends dir
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
                "servers" : [ // the following is an array of objects, each object represents a new server that will be spawned
                    {
                        "is_enabled" : true, // boolean
                        "host_name":"mail.test.com", // the hostname of the server as set by MX record
                        "max_size": 1000000, // maximum size of an email in bytes
                        "private_key_file":"/path/to/pem/file/test.com.key",  // full path to pem file private key
                        "public_key_file":"/path/to/pem/file/test.com.crt", // full path to pem file certificate
                        "timeout":180, // timeout in number of seconds before an idle connection is closed
                        "listen_interface":"127.0.0.1:25", // listen on ip and port
                        "start_tls_on":true, // supports the STARTTLS command?
                        "tls_always_on":false, // always connect using TLS? If true, start_tls_on will be false
                        "max_clients": 1000, // max clients at one time
                        "log_file":"/dev/stdout" // where to log to (currently ignored)
                    },
                    // the following is a second server, but listening on port 465 and always using TLS
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
                        "max_clients":500,
                        "log_file":"/dev/stdout"
                    }
                    // repeat as many servers as you need
                ]
            }
    }

The Json parser is very strict on syntax. If there's a parse error and it
doesn't give much clue, then test your syntax here:
http://jsonlint.com/#

Email Saving Backends
=====================

Backends provide for a modular way to save email and for the ability to
extend this functionality. They can be swapped in or out via the config. 
Currently, the server comes with two example backends: 

- dummy : used for testing purposes
- guerrilla_db_redis: example uses MySQL and Redis to store email, used on Guerrilla Mail

Releases
========

(Master branch - Release Candidate 1 for v1.6)
Large refactoring of the code. 
- Introduced "backends": modular architecture for saving email
- Issue: Use as a package in your own projects! https://github.com/flashmob/go-guerrilla/issues/20
- Issue: Do not include dot-suffix in emails https://github.com/flashmob/go-guerrilla/issues/24
- Logging functionality: logrus is now used for logging. Currently output is going to stdout
- Incompatible change: Config's allowed_hosts is now an array
- Incompatible change: The server's command is now a command called `guerrillad`
 

1.5.1 - 4nd Nov 2016 (Latest tagged release)
- Small optimizations to the way email is saved

1.5 - 2nd Nov 2016
- Fixed a DoS vulnerability, stop reading after an input limit is reached
- Fixed syntax error in Json goguerrilla.conf.sample
- Do not load certificates if SSL is not enabled
- check database back-end connections before starting

1.4 - 25th Oct 2016
- New Feature: multiple servers!
- Changed the configuration file format to support multiple servers,
this means that a new configuration file would need to be created form the
sample (goguerrilla.conf.sample)
- Organised code into separate files. Config is now strongly typed, etc
- Deprecated nginx proxy support


1.3 14th July 2016
- Number of saveMail workers added to config (GM_SAVE_WORKERS)
- convenience function for reading int values form config
- advertise PIPELINING
- added HELP command
- rcpt to host validation: now case insensitive and done earlier (after DATA)
- iconv switched to: go get gopkg.in/iconv.v1

1.2 1st July 2016
- Reload config on SIGHUP
- Write current process id (pid) to a file, /var/run/go-guerrilla.pid by default


Using Nginx as a proxy
======================
Note: This release temporarily does not have proxy support.
An issue has been opened to put back in https://github.com/flashmob/go-guerrilla/issues/30
Nginx can be used to proxy SMTP traffic for GoGuerrilla SMTPd

Why proxy SMTP with Nginx?

 *	Terminate TLS connections: (eg. Early Golang versions were not there yet when it came to TLS.)
 OpenSSL on the other hand, used in Nginx, has a complete implementation of TLS with familiar configuration.
 *	Nginx could be used for load balancing and authentication

 1.	Compile nginx with --with-mail --with-mail_ssl_module (most current nginx packages have this compiled already)

 2.	Configuration:


		mail {
	        server {
	                listen  15.29.8.163:25;
	                protocol smtp;
	                server_name  ak47.example.com;
	                auth_http smtpauth.local:80/auth.txt;
	                smtp_auth none;
	                timeout 30000;
	                smtp_capabilities "SIZE 15728640";

	                # ssl default off. Leave off if starttls is on
	                #ssl                  on;
	                ssl_certificate      /etc/ssl/certs/ssl-cert-snakeoil.pem;
	                ssl_certificate_key  /etc/ssl/private/ssl-cert-snakeoil.key;
	                ssl_session_timeout  5m;
	                # See https://mozilla.github.io/server-side-tls/ssl-config-generator/ Intermediate settings
	                ssl_protocols TLSv1 TLSv1.1 TLSv1.2;
	                ssl_ciphers 'ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-AES128-SHA256:ECDHE-RSA-AES128-SHA256:ECDHE-ECDSA-AES128-SHA:ECDHE-RSA-AES256-SHA384:ECDHE-RSA-AES128-SHA:ECDHE-ECDSA-AES256-SHA384:ECDHE-ECDSA-AES256-SHA:ECDHE-RSA-AES256-SHA:DHE-RSA-AES128-SHA256:DHE-RSA-AES128-SHA:DHE-RSA-AES256-SHA256:DHE-RSA-AES256-SHA:ECDHE-ECDSA-DES-CBC3-SHA:ECDHE-RSA-DES-CBC3-SHA:EDH-RSA-DES-CBC3-SHA:AES128-GCM-SHA256:AES256-GCM-SHA384:AES128-SHA256:AES256-SHA256:AES128-SHA:AES256-SHA:DES-CBC3-SHA:!DSS';
	                ssl_prefer_server_ciphers on;
	                # TLS off unless client issues STARTTLS command
	                starttls on;
	                proxy on;
	        }
		}

		http {

		    # Add somewhere inside your http block..
		    # make sure that you have added smtpauth.local to /etc/hosts
		    # What this block does is tell the above stmp server to connect
		    # to our golang server configured to run on 127.0.0.1:2525

		    server {
                    listen 15.29.8.163:80;
                    server_name 15.29.8.163 smtpauth.local;
                    root /home/user/http/auth/;
                    access_log off;
                    location /auth.txt {
                        add_header Auth-Status OK;
                        # where to find your smtp server?
                        add_header Auth-Server 127.0.0.1;
                        add_header Auth-Port 2525;
                    }

                }

		}




Starting / Command Line usage
==========================================================

All command line arguments are optional

	-config="goguerrilla.conf": Path to the configuration file
	 -if="": Interface and port to listen on, eg. 127.0.0.1:2525
	 -v="n": Verbose, [y | n]

Starting from the command line (example)

	/usr/bin/nohup /home/mike/goguerrilla -config=/home/mike/goguerrilla.conf 2>&1 &

This will place goguerrilla in the background and continue running

You may also put another process to watch your goguerrilla process and re-start it
if something goes wrong.

Testing STARTTLS

Use openssl:

    $ openssl s_client -starttls smtp -crlf -connect 127.0.0.1:2526


Benchmarking:
==========================================================

https://web.archive.org/web/20110725141905/http://www.jrh.org/smtp/index.html

Test 500 clients:
$ time smtp-source -c -l 5000 -t test@spam4.me -s 500 -m 5000 5.9.7.183

Authors
=======

Project Lead: 
-------------
Flashmob, GuerrillaMail.com, Contact: flashmob@gmail.com

Major Contributors: 
-------------------

* Reza Mohammadi https://github.com/remohammadi
* Jordan Schalm https://github.com/jordanschalm 

Thanks to:
----------
* https://github.com/dvcrn
* https://github.com/athoune

... and anyone else who opened an issue / sent a PR / gave suggestions!