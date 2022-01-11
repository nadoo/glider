#!/bin/sh

set -e

/bin/systemctl daemon-reload

if /bin/systemctl is-active --quiet glider@glider; then
    /bin/systemctl restart glider@glider
fi

if ! /bin/systemctl is-enabled --quiet glider@glider; then
    /bin/systemctl enable --now glider@glider;
fi