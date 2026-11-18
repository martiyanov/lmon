#!/bin/sh
# Включить NVIDIA по требованию (загрузить модуль).
set -eu
if [ "$(id -u)" -ne 0 ]; then exec sudo -- "$0" "$@"; fi
modprobe nvidia
modprobe nvidia_uvm 2>/dev/null || true
nvidia-smi
