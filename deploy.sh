#!/bin/bash

sudo systemctl stop blmonitor
sudo cp blmonitor /usr/local/bin
sudo cp spamcop-inject /usr/lib/dovecot/sieve-pipe/spamcop-inject
sudo cp blmonitor.service /etc/systemd/system
sudo systemctl daemon-reload
sudo systemctl restart blmonitor
