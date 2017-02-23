# Dashboard

The dashboard package gathers data about Guerrilla while it is running and provides an analytics web dashboard. To activate the dashboard, add it to your configuration file as specified in the example configuration.

## Backend

The backend is a Go package that collects and stores data from guerrillad, serves the dashboard to web clients, and updates clients with new analytics data over WebSockets. The backend uses [statik](https://github.com/rakyll/statik) to convert the `build` folder into a http-servable Go package. When deploying, the frontend should be built first, then the `statik` package should be created. An example of this process is in the `.travis.yml`.

## Frontend

The front-end is written in React and uses WebSockets to accept data from the backend and [Victory](https://formidable.com/open-source/victory/) to render charts. The `js` directory is an NPM module that contains all frontend code. All commands below should be run within the `js` directory.

To install frontend dependencies:
`npm install`

To build the frontend code:
`npm run build`

To run the HMR development server (serves frontend on port 3000 rather than through `dashboard` package):
`npm start`
