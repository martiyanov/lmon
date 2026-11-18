#!/bin/sh
# NVIDIA всегда загружена, но в простое — минимум питания и шума.
# Запуск: sudo scripts/nvidia-always-setup.sh [power_limit_watts]
set -eu
if [ "$(id -u)" -ne 0 ]; then
	echo "запусти с sudo: sudo $0 [power_limit_watts]" >&2
	exit 1
fi
power_limit=${1:-}

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

echo "== nvidia-modprobe (на случай CUDA/nvidia-smi без root) =="
if ! command -v nvidia-modprobe >/dev/null 2>&1; then
	apt-get update -qq
	DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nvidia-modprobe
fi

echo "== автозагрузка модуля =="
printf 'nvidia\n' >/etc/modules-load.d/nvidia.conf
cat /etc/modules-load.d/nvidia.conf

echo "== без persistenced (он держит GPU инициализированной) =="
systemctl disable --now nvidia-persistenced 2>/dev/null || true

echo "== проверка blacklist =="
if grep -RqsE '^[[:space:]]*blacklist[[:space:]]+nvidia([[:space:]]|$)' /etc/modprobe.d/ /lib/modprobe.d/ 2>/dev/null; then
	echo "ВНИМАНИЕ: nvidia заблокирован — уберите blacklist nvidia" >&2
fi

if [ "$need_reboot" -eq 1 ]; then
	echo
	echo "Перезагрузись и в MOK manager: Enroll MOK -> Continue -> Yes -> пароль."
	echo "Потом снова: sudo $0 ${power_limit:-}"
	exit 0
fi

echo "== загрузка модуля =="
modprobe nvidia
modprobe nvidia_uvm 2>/dev/null || true

echo "== persistence off: карта уходит в низкое энергопотребление в простое =="
nvidia-smi -pm 0 >/dev/null 2>&1 || true

if [ -n "$power_limit" ]; then
	echo "== power limit: ${power_limit} W =="
	nvidia-smi -pl "$power_limit"
fi

echo
nvidia-smi --query-gpu=name,power.draw,temperature.gpu,fan.speed,utilization.gpu,clocks.sm \
	--format=csv,noheader 2>/dev/null || nvidia-smi
echo
echo "Драйвер загружен постоянно. В простое RTX 3060 обычно 10-20 Вт и 0 об/мин,"
echo "если прошивка карты умеет zero-RPM. Если вентилятор шумит в простое —"
echo "ограничь мощность: sudo nvidia-smi -pl 120 (значение подбери под карту)."
echo "Полностью выключить: sudo ~/bin/nvidia-off.sh (выгрузит модуль)."
