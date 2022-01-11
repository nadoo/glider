#!/bin/sh

set -e

if /bin/systemctl is-active --quiet glider@glider; then
    /bin/systemctl stop glider@glider
fi

if /bin/systemctl is-enabled --quiet glider@glider; then
    /bin/systemctl disable --now glider@glider;
fi