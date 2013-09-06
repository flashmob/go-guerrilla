
GoGuerrilla
===========
An minimalist SMTP server written in [Go][1] for receiving large volumes of mail.

![Go Guerrilla](https://raw.github.com/flashmob/go-guerrilla/master/GoGuerrilla.png)

### What is GoGuerrilla?
It's a small SMTP server written in Go, for the purpose of receiving large volume of email. The purpose of this daemon is to grab the email, save it to the database and disconnect as quickly as possible. Written for use at guerrillaMail.com which processes tens of thousands of emails every hour.

A typical user of this software would probably want to customize the saveMail function for their own systems.


### Limitations of GoGuerrilla
This server does not attempt to filter HTML, check for spam or do any sender 
verification. These steps should be performed by other programs. 

The server does NOT send any email including bounces. This should be performed by a separate program.

As of Nov 2012, only a partial implementation of TLS is provided within Go. 
OpenSSL on the other hand, used in Nginx, has a complete implementation of
SSL v2/v3 and TLS protocols. See **Using Nginx as a Proxy (Option)** section below for more details.

### History and purpose
GoGuerrilla is a port of the original [Guerrilla-SMTPd][2] daemon written in PHP using an event-driven I/O library (libevent). It's not a direct port, although the purpose and functionality remains identical.

This Go version was made in order to take advantage of our new server with 8 cores. Not that the PHP version was taking much CPU anyway, it always stayed at about 1-5% despite guzzling down a ton of email every day...

As always, the bottleneck today is the network and secondary storage. It's highly probable that in the near future, secondary storage will become so fast that the I/O bottleneck will not be an issue. Prices of Solid State Drives are dropping fast, their speeds are rapidly increasing. So if the I/O bottleneck would disappear, it will be replaced by a new bottleneck, the CPU. 

To prepare for the CPU bottleneck, we need to be able to scale our software to multiple cores. Since PHP runs in a single process, it can only run on a single core. Sure, it would have been possible to use fork(), but that can get messy and doesn't play well with libevent. Also, it would have been possible to start an instance for each core and use a proxy to distribute the traffic to each instance, but that would make the system too complicated.

The most alluring aspect of Go are the [goroutines][3]! Using goroutines makes concurrent programming easy, clean and fun! Go programs can also take advantage of all your machine's multiple cores without much effort that you would otherwise need with forking or managing your event loop callbacks, etc. Golang solves the C10K problem in a very interesting way
 http://en.wikipedia.org/wiki/C10k_problem

If you do invite GoGuerrilla into your system, please remember to feed it with lots of spam - spam is what it likes best!

Getting started
===========================

## Requirements

GoGuerrilla requires:

 - Go
 - MySQL

## Manual Install

### Prerequisite Go Libraries
To build, you will need to install the following Go libs:

```bash
go get github.com/ziutek/mymysql/thrsafe
go get github.com/ziutek/mymysql/autorc
go get github.com/ziutek/mymysql/godrv
go get github.com/sloonz/go-iconv
go get github.com/garyburd/redigo/redis
go get github.com/sloonz/go-qprintable
```

### MySQL Database
Setup the following schema and tables (schema name can be modified to suit):

Option 1: via shell:

```bash
mysql < scripts/schema_init.sql -u<your MySQL username> -p<your MySQL password>
```

Option 2: via copy-paste to your favorite SQL editor:
```SQL
CREATE SCHEMA IF NOT EXISTS `go_guerrilla` DEFAULT CHARACTER SET utf8;

USE `go_guerrilla`;

CREATE TABLE IF NOT EXISTS `mail_queue` (
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
) ENGINE=InnoDB  DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `_settings` (
  `setting_name` varchar(128) character set latin1 NOT NULL,
  `setting_value` int(11) NOT NULL,
   PRIMARY KEY  (`setting_name`)
) ENGINE=InnoDB  DEFAULT CHARSET=utf8;
```

### Configuration
The configuration is in strict JSON format and a sample configuration named ```goguerrilla.conf.sample``` has been provided. To begin, copy the configuration as a file named ```goguerilla.conf```, the default configuration file that GoGuerrilla will seek at runtime:

```bash
cp goguerrilla.conf.sample goguerrilla.conf
```

then edit ```goguerrilla.conf``` with your favorite editor:

```bash
vim goguerrilla.conf
```

#### Configuration File Format
Here is an annotated configuration:

```
{
    "GM_ALLOWED_HOSTS":"example.com,foo.com", 	// accept mail for these domains
    "GM_MAIL_TABLE":"mail_queue", 			    // name of email table
    "GM_PRIMARY_MAIL_HOST":"mail.example.com", 	// given in the SMTP greeting
    "GSMTP_HOST_NAME":"mail.example.com", 		// given in the SMTP greeting
    "GSMTP_LOG_FILE":"/dev/stdout", 			// not used yet
    "GSMTP_MAX_SIZE":"131072", 					// max size of DATA command
    "GSMTP_PRV_KEY":"certs/example.com.key", 	// private key for TLS
    "GSMTP_PUB_KEY":"certs/example.com.pem", 	// public key for TLS
    "GSMTP_TIMEOUT":"100", 						// tcp connection timeout
    "GSMTP_VERBOSE":"N", 						// set to Y for debugging
    "GSTMP_LISTEN_INTERFACE":"127.0.0.1:25",	// ip:port for daemon service
    "MYSQL_DB":"go_guerrilla", 					// MySQL database name
    "MYSQL_HOST":"127.0.0.1:3306", 				// MySQL host
    "MYSQL_USER":"go_guerrilla", 				// MySQL username
    "MYSQL_PASS":"$ecure1t", 					// MySQL password
    "GM_MAX_CLIENTS":"500", 					// max concurrent connections
	"NGINX_AUTH_ENABLED":"N",      				// "Y" or "N" -- see nginx section
	"NGINX_AUTH":"127.0.0.1:8025", 				// ip:port for nginx authentication
								   				// see nginx section for details
    "SGID":"508",								// groupid for user from /etc/passwd
	"GUID":"504" 								// uid from /etc/passwd
}
```

## Compile/Run

### Basic Usage
```bash
go run ./go_guerrilla.go
```

### Arguments

 - -config: *path to the configuration file*
	- -config="mygsmtp.conf"
 - -if: *interface and port to listen on*
	- -if="127.0.0.1:2525"
 - -v: *verbose, [y | n]*
	- -v="N"

### Example: GoGuerrilla as a long-running process
To place goguerrilla in the background and continue running:
```bash
/usr/bin/nohup $HOME/goguerrilla -config=$HOME/goguerrilla.conf 2>&1 &
```
You may also put another process to watch your goguerrilla process and re-start it if something goes wrong.

Using Nginx as a Proxy (Option)
===============================
[Nginx][4] can be used to proxy SMTP traffic for GoGuerrilla. This allows GoGuerrilla to work around limitations of TLS within Go (as of Nov 2012) as well as offering front-end load balancing and authentication of the service in the future.

### Compile
Compile nginx from source with --with-mail --with-mail_ssl_module

### Example nginx.conf
```
mail {
    auth_http 127.0.0.1:8025/; # This is the URL to GoGuerrilla's service 
                               # which tells Nginx where to proxy the traffic to 								
    server {
            listen  0.0.0.0:25;
            protocol smtp;
            server_name  gsmtp.example.com;

            smtp_auth none;
            timeout 30000;
			smtp_capabilities "SIZE 15728640"; # GoGuerrilla's max DATA size
			
			# ssl default off -- leave off if starttls is on
            ssl_certificate      /etc/ssl/certs/ssl-cert-snakeoil.pem;
            ssl_certificate_key  /etc/ssl/private/ssl-cert-snakeoil.key;
            ssl_session_timeout  5m;
            ssl_protocols  SSLv2 SSLv3 TLSv1;
            ssl_ciphers  HIGH:!aNULL:!MD5;
            ssl_prefer_server_ciphers   on;
			# TLS off unless client issues STARTTLS command
            starttls on;
            proxy on;
    }
}
```
				
Assuming that GoGuerrilla has the following configuration settings:

	"GSMTP_MAX_SIZE"		  "15728640",
	"NGINX_AUTH_ENABLED":     "Y",
	"NGINX_AUTH":             "127.0.0.1:8025", 

  [1]: http://www.golang.org
  [2]: https://github.com/flashmob/Guerrilla-SMTPd
  [3]: http://golang.org/doc/effective_go.html#goroutines
  [4]: http://nginx.org/
