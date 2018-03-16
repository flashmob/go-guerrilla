
[![Build Status](https://travis-ci.org/flashmob/go-guerrilla.svg?branch=master)](https://travis-ci.org/flashmob/go-guerrilla)

Go-Guerrilla SMTP Daemon
====================

A lightweight SMTP server written in Go, made for receiving large volumes of mail.
To be used as a package in your Go project, or as a stand-alone daemon by running the "guerrillad" binary.

Supports MySQL and Redis out-of-the-box, with many other vendor provided _processors_,
such as [MailDir](https://github.com/flashmob/maildir-processor) and even [FastCGI](https://github.com/flashmob/fastcgi-processor)! 
See below for a list of available processors.

![Go Guerrilla](/GoGuerrilla.png)

### What is Go-Guerrilla?

It's an SMTP server written in Go, for the purpose of receiving large volumes of email.
It started as a project for GuerrillaMail.com which processes millions of emails every day,
and needed a daemon with less bloat & written in a more memory-safe language that can 
take advantage of modern multi-core architectures.

The purpose of this daemon is to grab the email, save it,
and disconnect as quickly as possible, essentially performing the services of a
Mail Transfer Agent (MTA) without the sending functionality.

The software also includes a modular backend implementation, which can extend the email
processing functionality to whatever needs you may require. We refer to these modules as 
"_Processors_". Processors can be chained via the config to perform different tasks on 
received email, or to validate recipients.

See the list of available _Processors_ below.

For more details about the backend system, see the:
[Backends, configuring and extending](https://github.com/flashmob/go-guerrilla/wiki/Backends,-configuring-and-extending) page.

### License

The software is using MIT License (MIT) - contributors welcome.

### Features

#### Main Features

- Multi-server. Can spawn multiple servers, all sharing the same backend
for saving email.
- Config hot-reloading. Add/Remove/Enable/Disable servers without restarting. 
Reload TLS configuration, change most other settings on the fly.
- Graceful shutdown: Minimise loss of email if you need to shutdown/restart.
- Be a gentleman to the garbage collector: resources are pooled & recycled where possible.
- Modular [Backend system](https://github.com/flashmob/go-guerrilla/wiki/Backends,-configuring-and-extending) 
- Modern TLS support (STARTTLS or SMTPS).
- Can be [used as a package](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package) in your Go project. 
Get started in just a few lines of code!
- [Fuzz tested](https://github.com/flashmob/go-guerrilla/wiki/Fuzz-testing). 
[Auto-tested](https://travis-ci.org/flashmob/go-guerrilla). Battle Tested.

#### Backend Features

- Arranged as workers running in parallel, using a producer/consumer type structure, 
 taking advantage of Go's channels and go-routines. 
- Modular [backend system](https://github.com/flashmob/go-guerrilla/wiki/Backends,-configuring-and-extending)
 structured using a [decorator-like pattern](https://en.wikipedia.org/wiki/Decorator_pattern) which allows the chaining of components (a.k.a. _Processors_) via the config.  
- Different ways for processing / delivering email: Supports MySQL and Redis out-of-the box, many other 
vendor provided processors available.

### Roadmap / Contributing & Bounties

Pull requests / issue reporting & discussion / code reviews always 
welcome. To encourage more pull requests, we are now offering bounties. 

Take a look at our [Bounties and Roadmap](https://github.com/flashmob/go-guerrilla/wiki/Roadmap-and-Bounties) page!


Getting started
===========================

(Assuming that you have GNU make and latest Go on your system)

#### Dependencies

Go-Guerrilla uses [Glide](https://github.com/Masterminds/glide) to manage 
dependencies. If you have glide installed, just run `glide install` as usual.
 
You can also run `$ go get ./..` if you don't want to use glide, and then run `$ make test`
to ensure all is good.

To build the binary run:

```
$ make guerrillad
```

This will create a executable file named `guerrillad` that's ready to run.
See the [build notes](https://github.com/flashmob/go-guerrilla/wiki/Build-Notes) for more details.

Next, copy the `goguerrilla.conf.sample` file to `goguerrilla.conf.json`. 
You may need to customize the `pid_file` setting to somewhere local, 
and also set `tls_always_on` to false if you don't have a valid certificate setup yet. 

Next, run your server like this:

`$ ./guerrillad serve`

The configuration options are detailed on the [configuration page](https://github.com/flashmob/go-guerrilla/wiki/Configuration). 
The main takeaway here is:

The default configuration uses 3 _processors_, they are set using the `save_process` 
config option. Notice that it contains the following value: 
`"HeadersParser|Header|Debugger"` - this means, once an email is received, it will
first go through the `HeadersParser` processor where headers will be parsed.
Next, it will go through the `Header` processor, where delivery headers will be added.
Finally, it will finish at the `Debugger` which will log some debug messages.

Where to go next?

- Try setting up an [example configuration](https://github.com/flashmob/go-guerrilla/wiki/Configuration-example:-save-to-Redis-&-MySQL) 
which saves email bodies to Redis and metadata to MySQL.
- Try importing some of the 'vendored' processors into your project. See [MailDiranasaurus](https://github.com/flashmob/maildiranasaurus)
as an example project which imports the [MailDir](https://github.com/flashmob/maildir-processor) and [FastCGI](https://github.com/flashmob/fastcgi-processor) processors.
- Try hacking the source and [create your own processor](https://github.com/flashmob/go-guerrilla/wiki/Backends,-configuring-and-extending).
- Once your daemon is running, you might want to stup [log rotation](https://github.com/flashmob/go-guerrilla/wiki/Automatic-log-file-management-with-logrotate).



Use as a package
============================
Go-Guerrilla can be imported and used as a package in your Go project.

### Quickstart


#### 1. Import the guerrilla package
```go
import (
    "github.com/flashmob/go-guerrilla"
)


```

You may use ``$ go get ./...`` to get all dependencies, also Go-Guerrilla uses 
[glide](https://github.com/Masterminds/glide) for dependency management.

#### 2. Start a server

This will start a server with the default settings, listening on `127.0.0.1:2525`


```go

d := guerrilla.Daemon{}
err := d.Start()

if err == nil {
    fmt.Println("Server Started!")
}
```

`d.Start()` *does not block* after the server has been started, so make sure that you keep your program busy.

The defaults are: 
* Server listening to 127.0.0.1:2525
* use your hostname to determine your which hosts to accept email for
* 100 maximum clients
* 10MB max message size 
* log to Stderror, 
* log level set to "`debug`"
* timeout to 30 sec 
* Backend configured with the following processors: `HeadersParser|Header|Debugger` where it will log the received emails.

Next, you may want to [change the interface](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#starting-a-server---custom-listening-interface) (`127.0.0.1:2525`) to the one of your own choice.

#### API Documentation topics

Please continue to the [API documentation](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package) for the following topics:


- [Suppressing log output](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#starting-a-server---suppressing-log-output)
- [Custom listening interface](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#starting-a-server---custom-listening-interface)
- [What else can be configured](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#what-else-can-be-configured)
- [Backends](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#backends)
    - [About the backend system](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#about-the-backend-system)
    - [Backend Configuration](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#backend-configuration)
    - [Registering a Processor](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#registering-a-processor)
- [Loading config from JSON](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#loading-config-from-json)
- [Config hot-reloading](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#config-hot-reloading)
- [Logging](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#logging-stuff)
- [Log re-opening](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#log-re-opening)
- [Graceful shutdown](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#graceful-shutdown)
- [Pub/Sub](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#pubsub)
- [More Examples](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#more-examples)

Use as a Daemon
==========================================================

### Manual for using from the command line

- [guerrillad command](https://github.com/flashmob/go-guerrilla/wiki/Running-from-command-line#guerrillad-command)
    - [Starting](https://github.com/flashmob/go-guerrilla/wiki/Running-from-command-line#starting)
    - [Re-loading configuration](https://github.com/flashmob/go-guerrilla/wiki/Running-from-command-line#re-loading-the-config)
    - [Re-open logs](https://github.com/flashmob/go-guerrilla/wiki/Running-from-command-line#re-open-log-file)
    - [Examples](https://github.com/flashmob/go-guerrilla/wiki/Running-from-command-line#examples)

### Other topics

- [Using Nginx as a proxy](https://github.com/flashmob/go-guerrilla/wiki/Using-Nginx-as-a-proxy)
- [Testing STARTTLS](https://github.com/flashmob/go-guerrilla/wiki/Running-from-command-line#testing-starttls)
- [Benchmarking](https://github.com/flashmob/go-guerrilla/wiki/Profiling#benchmarking)


Email Processing Backend
=====================

The main job of a Go-Guerrilla backend is to validate recipients and deliver emails. The term
"delivery" is often synonymous with saving email to secondary storage.

The default backend implementation manages multiple workers. These workers are composed of 
smaller components called "Processors" which are chained using the config to perform a series of steps.
Each processor specifies a distinct feature of behaviour. For example, a processor may save
the emails to a particular storage system such as MySQL, or it may add additional headers before 
passing the email to the next _processor_.

To extend or add a new feature, one would write a new Processor, then add it to the config.
There are a few default _processors_ to get you started.


### Included Processors

| Processor | Description |
|-----------|-------------|
|Compressor|Sets a zlib compressor that other processors can use later|
|Debugger|Logs the email envelope to help with testing|
|Hasher|Processes each envelope to produce unique hashes to be used for ids later|
|Header|Add a delivery header to the envelope|
|HeadersParser|Parses MIME headers and also populates the Subject field of the envelope|
|MySQL|Saves the emails to MySQL.|
|Redis|Saves the email data to Redis.|
|GuerrillaDbRedis|A 'monolithic' processor used at Guerrilla Mail; included for example

### Available Processors

The following processors can be imported to your project, then use the
[Daemon.AddProcessor](https://github.com/flashmob/go-guerrilla/wiki/Using-as-a-package#registering-a-processor) function to register, then add to your config.

| Processor | Description |
|-----------|-------------|
|[MailDir](https://github.com/flashmob/maildir-processor)|Save emails to a maildir. [MailDiranasaurus](https://github.com/flashmob/maildiranasaurus) is an example project|
|[FastCGI](https://github.com/flashmob/fastcgi-processor)|Deliver email directly to PHP-FPM or a similar FastCGI backend.|
|[WildcardProcessor](https://github.com/DevelHell/wildcard-processor)|Use wildcards for recipients host validation.|

Have a processor that you would like to share? Submit a PR to add it to the list!

Releases
========

Current release: 1.5.1 - 4th Nov 2016

Next Planned release: 2.0.0 - TBA

See our [change log](https://github.com/flashmob/go-guerrilla/wiki/Change-Log) for change and release history


Using Nginx as a proxy
======================

For such purposes as load balancing, terminating TLS early,
 or supporting SSL versions not supported by Go (highly not recommenced if you
 want to use older SSL versions), 
 it is possible to [use NGINX as a proxy](https://github.com/flashmob/go-guerrilla/wiki/Using-Nginx-as-a-proxy).



Credits
=======

Project Lead: 
-------------
Flashmob, GuerrillaMail.com, Contact: flashmob@gmail.com

Major Contributors: 
-------------------

* Reza Mohammadi https://github.com/remohammadi
* Jordan Schalm https://github.com/jordanschalm 
* Philipp Resch https://github.com/dapaxx

Thanks to:
----------
* https://github.com/dvcrn
* https://github.com/athoune
* https://github.com/Xeoncross

... and anyone else who opened an issue / sent a PR / gave suggestions!
