#!/bin/sh
# Один раз: разрешить загрузку NVIDIA при Secure Boot и включить on-demand.
# Запускать: sudo scripts/nvidia-ondemand-setup.sh
set -eu
if [ "$(id -u)" -ne 0 ]; then
	echo "запусти с sudo: sudo $0" >&2
	exit 1
fi

echo "== Secure Boot =="
sb=$(mokutil --sb-state 2>&1 || true)
echo "$sb"
need_reboot=0
if echo "$sb" | grep -q "SecureBoot enabled"; then
	key=/var/lib/shim-signed/mok/MOK.der
	if [ ! -f "$key" ]; then
		echo "нет $key — Secure Boot не настроен для DKMS" >&2
		exit 1
	fi
	if mokutil --test-key "$key" 2>/dev/null | grep -q "is already enrolled"; then
		echo "MOK уже зарегистрирован"
	else
		echo "MOK не зарегистрирован — импортирую ключ"
		mokutil --import "$key"
		need_reboot=1
	fi
fi

echo "== nvidia-modprobe (загрузка по требованию) =="
if ! command -v nvidia-modprobe >/dev/null 2>&1; then
	apt-get update -qq
	DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nvidia-modprobe
fi
command -v nvidia-modprobe

echo "== проверка автозагрузки =="
if grep -RqsE '^[[:space:]]*nvidia[[:space:]]*$' /etc/modules-load.d/ 2>/dev/null; then
	echo "ВНИМАНИЕ: nvidia есть в /etc/modules-load.d — для on-demand уберите"
fi
systemctl disable --now nvidia-persistenced 2>/dev/null || true
if grep -RqsE '^[[:space:]]*blacklist[[:space:]]+nvidia([[:space:]]|$)' /etc/modprobe.d/ /lib/modprobe.d/ 2>/dev/null; then
	echo "ВНИМАНИЕ: nvidia заблокирован — on-demand не сработает, уберите blacklist nvidia"
fi

if [ "$need_reboot" -eq 1 ]; then
	echo
	echo "Перезагрузись и в MOK manager: Enroll MOK -> Continue -> Yes -> пароль."
	echo "Потом снова: sudo $0"
	exit 0
fi

echo "== загрузка модуля =="
modprobe nvidia
modprobe nvidia_uvm 2>/dev/null || true
nvidia-smi
echo
echo "OK. Дальше:"
echo "  lmon --gpu-list"
echo "  lmon --gpu --gputemp --gpu-index 0"
