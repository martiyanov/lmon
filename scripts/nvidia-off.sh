#!/bin/sh
# Выгрузить NVIDIA, если GPU никто не использует.
set -eu
if [ "$(id -u)" -ne 0 ]; then exec sudo -- "$0" "$@"; fi
apps=$(nvidia-smi --query-compute-apps=pid,process_name --format=csv,noheader 2>/dev/null || true)
if [ -n "$apps" ]; then
	echo "GPU используется, выгружать нельзя:" >&2
	echo "$apps" >&2
	exit 1
fi
for m in nvidia_uvm nvidia_drm nvidia_modeset nvidia; do
	modprobe -r "$m" 2>/dev/null || true
done
if lsmod | grep -q '^nvidia'; then
	echo "модуль не выгрузился:" >&2
	lsmod | grep nvidia >&2
	exit 1
fi
echo "NVIDIA выгружена (карта выключена до следующего запуска)"
