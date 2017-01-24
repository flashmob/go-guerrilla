#!/bin/bash

# Build frontend to `dashboard/js/build`
# cd dashboard/js && npm i && cd ../../
cd dashboard/js && npm run build && cd ../../
# Build statik file system in `dashboard/statik`
statik -src=dashboard/js/build -dest=dashboard
# Build guerrillad
make guerrillad
