
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


### So what's the story?

Originally, Guerrilla Mail was running the Exim mail server, and the emails
were fetched using POP.

This proved to be inefficient and unreliable, so the system was replaced in 
favour of piping the emails directly in to a PHP script.

Soon, the piping solution  became a problem too; it required a new process to 
be started for each arriving email, and it also required a new database 
connection every time. 

So, how was the bottleneck eliminated? Conveniently, PHP has a socket 
library which means we can use PHP to quickly prototype a simple SMTP server.
If the server runs as a daemon, then the system doesn't need to launch a new 
process for each incoming email. It also doesn't need to run and insane amount 
of checks for each connection (eg, NS Lookups, white-lists, black-lists, SPF
domain keys, Spam Assassin, etc).

We only need to open a single database connection and a single process can be 
re-used indefinitely. The PHP server was able to multiplex simultaneous 
connections without the need for forking/threading thanks to socket_select()

Therefore, we could receive, process and store email all in the one process.
The performance improvement has been dramatic. 

However, a few months later, the volume of email increased again and
this time our server's CPU was under pressure. You see, the problem was that
the SMTP server was checking the sockets in a loop all the time. This is fine
if you have a few sockets, but horrible if you have 1000! A common way to solve
this is to have a connection per thread - and a thread can sleep while it is
waiting for a socket, not polling all the time.

Threads are great if you have many CPU cores, but our server only had two.
Besides, our process was I/O bound - there's a better way than to block.
So how to solve this one? 

Instead of polling/blocking in an infinite loop to see if there are any sockets to be 
operated, it was more efficient to be notified when the sockets are ready.
This Wikipedia entry explains it best, 
http://en.wikipedia.org/wiki/Asynchronous_I/O see "Select(/poll) loops"

Luck had it that an extension is available for PHP! One night later, and version 2 was made
to use libevent http://pecl.php.net/package/libevent

The improvement was superb. The load average has decreased
substantially, freeing the CPU for other tasks. It was even surprising to see that
PHP could handle so many emails.

Fast forward to 2012, to where we are now. Golang 1.0 was released, so it was
decided to give it a go.

As a language, Go is brilliant for writing server back-ends, with support for concurrency. 
There's a nice tutorial for getting started on the Golang.org website. 
There were a lot of great discoveries along the way, particularly the 'channels' 
in Go can be simple, yet very powerful and the defer statement is quite convenient. 
It looks like there's a wealth of packages available for
almost everything, including MySQL.



Getting started
===========================

To build, you will need to install the following Go libs:

	$ go get github.com/ziutek/mymysql/thrsafe
	$ go get github.com/ziutek/mymysql/autorc
	$ go get github.com/ziutek/mymysql/godrv
	$ go get github.com/sloonz/go-iconv
	$ go get github.com/garyburd/redigo/redis

Rename goguerrilla.conf.sample to goguerrilla.conf

Setup the following table:
(The vanilla saveMail function also uses Redis)

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
	    "SGID":"508",// group id of the user from /etc/passwd
		"GUID":"504" // uid from /etc/passwd
	}

Using Nginx as a proxy
=========================================================
Nginx can be used to proxy SMTP traffic for GoGuerrilla SMTPd

Why proxy SMTP?

 *	Terminate TLS connections: Golang is not there yet when it comes to TLS.
At present, only a partial implementation of TLS is provided (as of Nov 2012). 
OpenSSL on the other hand, used in Nginx, has a complete implementation of
SSL v2/v3 and TLS protocols.
 *	Could be used for load balancing and authentication in the future.

 1.	Compile nginx with --with-mail --with-mail_ssl_module

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
	                ssl_protocols  SSLv2 SSLv3 TLSv1;
	                ssl_ciphers  HIGH:!aNULL:!MD5;
	                ssl_prefer_server_ciphers   on;
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