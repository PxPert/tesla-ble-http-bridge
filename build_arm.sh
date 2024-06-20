#!/bin/bash

docker buildx build  -t pxpert/tesla-ble-http-bridge:latest --build-arg="GOARM=5" --platform linux/arm/v6 . --push
