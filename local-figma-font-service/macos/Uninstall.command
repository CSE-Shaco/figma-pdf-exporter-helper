#!/bin/bash
set -euo pipefail
source "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)/lifecycle.sh"
font_uninstall
