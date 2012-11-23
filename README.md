
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


### History and purpose

GoGuerrilla is a port of the original 'Guerrilla' SMTP daemon written in PHP using
an event-driven I/O library (libevent)

https://github.com/flashmob/Guerrilla-SMTPd

It's not a direct port, although the purpose and functionality remains identical.

This Go version was made in order to take advantage of our new server with 8 cores. 
Not that the PHP version was taking much CPU anyway, it always stayed at about 1-5%
despite guzzling down a ton of email every day...

As always, the bottleneck today is the network and secondary storage. It's highly probable
that in the near future, secondary storage will become so fast that the I/O bottleneck
will not be an issue. Prices of Solid State Drives are dropping fast, their speeds are rapidly
increasing. So if the I/O bottleneck would disappear, it will be replaced by a new bottleneck,
the CPU. 

To prepare for the CPU bottleneck, we need to be able to scale our software to multiple cores.
Since PHP runs in a single process, it can only run on a single core. Sure, it would
have been possible to use fork(), but that can get messy and doesn't play well with
libevent. Also, it would have been possible to start an instance for each core and
use a proxy to distribute the traffic to each instance, but that would make the system too
 complicated.

The most alluring aspect of Go are the Goroutines! It makes concurrent programming
easy, clean and fun! Go programs can also take advantage of all your machine's multiple 
cores without much effort that you would otherwise need with forking or managing your
event loop callbacks, etc. Golang solves the C10K problem in a very interesting way
 http://en.wikipedia.org/wiki/C10k_problem

If you do invite GoGuerrilla in to your system, please remember to feed it with lots
of spam - spam is what it likes best!

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

You may also put another process to watch your goguerrilla process and re-start it
if something goes wrong.