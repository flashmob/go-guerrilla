# Dashboard

The dashboard package gathers data about Guerrilla while it is running 
and provides an analytics web dashboard. To activate the dashboard, checkout
the dashboard branch, then build it, then edit your configuration 
file as specified in the example configuration.

tl/dr

```
$ git checkout dashboard
$ cd dashboard/js
$ npm install
$ npm run build
$ cd ..
$ statik -src=./js/build
$ cd ..
$ make guerrillad
```

Then see the Config section below how to enable it!

## The Backend

The backend is a Go package that collects and stores data from guerrillad, 
serves the dashboard to web clients, and updates clients with new analytics data 
over WebSockets. 

The backend uses [statik](https://github.com/rakyll/statik) to convert the `build` 
folder into a http-servable Go package. When deploying, the frontend should be 
built first, then the `statik` package should be created. 
An example of this process is in the `.travis.yml`.

`To build the statik Go package, cd to the `dashboard` dir, then run
 
 `statik -src=./js/build` 

## The Frontend

The front-end is written in React and uses WebSockets to accept data 
from the backend and [Victory](https://formidable.com/open-source/victory/) to render charts. 
The `js` directory is an NPM module that contains all frontend code. 
All commands below should be run within the `js` directory.

To install frontend dependencies:
`npm install`

To build the frontend code:
`npm run build`

To run the HMR development server (serves frontend on port 3000 rather than through `dashboard` package):
`npm start`

## Config

Add `dashboard` to your goguerrilla.json config file

```
"dashboard": {
    "is_enabled": true,
    "listen_interface": ":8081",
    "tick_interval": "5s",
    "max_window": "24h",
    "ranking_aggregation_interval": "6h"
  }
```

## Security considerations

Warning: The dashboard does not have any authentication. It is also served over HTTP.

Assuming that the host will open the dashboard http port only to the local network or VPN. 
However, if you need to access the dashboard securely from a remote connection and
don't have a VPN, then maybe an SSH tunnel could do:

`ssh you@example.com -L 8081:127.0.0.1:8081 -N`

Then point your browser to http://127.0.0.1:8081