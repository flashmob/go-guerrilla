
Go-Guerrilla SMTPd
====================

An minimalist SMTP server written in Go, made for receiving large volumes of mail.

![Go Guerrilla](https://raw.github.com/flashmob/go-guerrilla/master/GoGuerrilla.png)

### What is Go Guerrilla SMTPd?

It's a small SMTP server written in Go, for the purpose of receiving large volume of email.
Written for GuerrillaMail.com which processes tens of thousands of emails
every hour.

The purpose of this daemon is to grab the email, save it to the database
and disconnect as quickly as possible.

A typical user of this software would probably want to customize the saveMail function for
their own systems.

This server does not attempt to filter HTML, check for spam or do any sender 
verification. These steps should be performed by other programs.
The server does NOT send any email including bounces. This should
be performed by a separate program.

The software is using MIT License (MIT) - contributors welcome.

### Roadmap / Contributing & Bounties

Wow, this project did not expect to get so many stars! 
However, not that much pull requests...
To encourage more pull requests, we are now offering bounties funded 
from our bitcoin donation address:

`1grr11aWtbsyMUeB4EGfHvTuu7eFzkJ4A`

So far we have the following bounties:

- Client Pooling: When a client is finished, it should be placed into a 
pool instead of being destroyed. Looking for a idiomatic 
Go solution with channels. (0.5 BTC for a successful merge)

- Modularize: Ability for the server to be used as a module. If it used
as a module, the new program would be able to start several servers on 
different ports, would be possible to specify a config file for 
each server, and specify its own saveMail function (otherwise, revert to
default). Ideas on how to best refactor this welcome too. 
(0.5 BTC for a successful merge)

- Analytics: A web based admin panel that displays live statistics,
including the number of clients, memory usage, graph the number of
connections/bytes/memory used for the last 24h. 
Show the top senders by: IP, by domain & by HELO message. 
Using websocket via https & password protected. 
(1 BTC for a successful merge)

Contact by opening an issue on Github you need more clarification / details.
Also, welcome your suggestions for adding things to this Roadmap.

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

To build, you will need to install the following Go libs:

	$ go get github.com/ziutek/mymysql/thrsafe
	$ go get github.com/ziutek/mymysql/autorc
	$ go get github.com/ziutek/mymysql/godrv
	$ go get gopkg.in/iconv.v1
	$ go get github.com/garyburd/redigo/redis

Rename goguerrilla.conf.sample to goguerrilla.conf

By default, the saveMail() function saves the meta-data of an email
into MySQL while the body is saved in Redis.

If you want to use the default saveMail() function, setup the following table
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
backend fits for you.


Configuration
============================================
The configuration is in strict JSON format. Here is an annotated configuration.
Copy goguerrilla.conf.sample to goguerrilla.conf


	{
	    "GM_ALLOWED_HOSTS":"example.com,sample.com,foo.com,bar.com", // which domains accept mail
	    "GM_MAIL_TABLE":"new_mail", // name of new email table
	    "GM_PRIMARY_MAIL_HOST":"mail.example.com", // given in the SMTP greeting
	    "GSMTP_HOST_NAME":"mail.example.com", // given in the SMTP greeting
	    "GSMTP_LOG_FILE":"/dev/stdout", // not used yet
	    "GSMTP_MAX_SIZE":"131072", // max size of DATA command
	    "GSMTP_PRV_KEY":"/etc/ssl/private/example.com.key", // private key for TLS
	    "GSMTP_PUB_KEY":"/etc/ssl/certs/example.com.crt", // public key for TLS
	    "GSMTP_TIMEOUT":"100", // tcp connection timeout
	    "GSMTP_VERBOSE":"N", // set to Y for debugging
	    "GSTMP_LISTEN_INTERFACE":"5.9.7.183:25",
	    "MYSQL_DB":"gmail_mail", // database name
	    "MYSQL_HOST":"127.0.0.1:3306", // database connect
	    "MYSQL_PASS":"$ecure1t", // database connection pass
	    "MYSQL_USER":"gmail_mail", // database username
	    "GM_MAX_CLIENTS":"500", // max clients that can be handled
		"NGINX_AUTH_ENABLED":"N",// Y or N
		"NGINX_AUTH":"127.0.0.1:8025", // If using Nginx proxy, choose an ip and port to serve Auth requsts for Nginx
	    "PID_FILE":		  "/var/run/go-guerrilla.pid",
	    "GM_SAVE_WORKERS : "3" // how many workers saving email to the storage
	}

Releases
=========================================================

1.3
- Number of saveMail workers added to config (GM_SAVE_WORKERS) 
- convenience function for reading int values form config
- advertise PIPELINING
- added HELP command
- rcpt to host validation: now case insensitive and done earlier (after DATA)
- iconv switched to: go get gopkg.in/iconv.v1

1.2
- Reload config on SIGHUP
- Write current process id (pid) to a file, /var/run/go-guerrilla.pid by default


Using Nginx as a proxy
=========================================================
Nginx can be used to proxy SMTP traffic for GoGuerrilla SMTPd

Why proxy SMTP with Nginx?

 *	Terminate TLS connections: Early Golang was not there yet when it came to TLS.
 OpenSSL on the other hand, used in Nginx, has a complete implementation of TLS with familiar configuration.
 *	Nginx could be used for load balancing and authentication

 1.	Compile nginx with --with-mail --with-mail_ssl_module (most current nginx packages have this compiled already)

 2.	Configuration:


		mail {
	        auth_http 127.0.0.1:8025/; # This is the URL to GoGuerrilla's http service which tells Nginx where to proxy the traffic to
	        server {
	                listen  15.29.8.163:25;
	                protocol smtp;
	                server_name  ak47.example.com;
	
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


Assuming that Guerrilla SMTPd has the following configuration settings:

	"GSMTP_MAX_SIZE"		  "15728640",
	"NGINX_AUTH_ENABLED":     "Y",
	"NGINX_AUTH":             "127.0.0.1:8025", 


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

Benchmarking:
==========================================================

http://www.jrh.org/smtp/index.html
Test 500 clients:
$ time smtp-source -c -l 5000 -t test@spam4.me -s 500 -m 5000 5.9.7.183
