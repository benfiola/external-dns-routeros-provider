#!/bin/sh
set -e
python -m venv /venv
. /venv/bin/activate
cd /workspaces/external-dns-mikrotik-webhook
pip install -e ".[dev]"