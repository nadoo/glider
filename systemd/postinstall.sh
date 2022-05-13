#!/bin/sh

set -e

if test ! -f "/etc/glider/glider.conf"; then
    cp /etc/glider/glider.conf.example /etc/glider/glider.conf
fi

/bin/systemctl daemon-reload

if /bin/systemctl is-active --quiet glider@glider; then
    /bin/systemctl restart glider@glider
fi

if ! /bin/systemctl is-enabled --quiet glider@glider; then
    /bin/systemctl enable --now glider@glider;
fi
