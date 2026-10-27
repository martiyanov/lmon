#!/usr/bin/env zsh
# lmon — минимальный монитор: заголовок закреплён сверху, значения ниже
# Колонки: N — пинг внешнего хоста, G — пинг шлюза, L — load average,
#          M — память (%), T — температура CPU (°C)
# Настройки: LMON_TARGET, LMON_GW, LMON_INTERVAL (сек, по умолчанию 1)
# Только цифры, пробелы и ASCII-дефисы — никаких спецсимволов.

lmon() {
  emulate -L zsh

  if [[ "$1" == help || "$1" == -h || "$1" == --help ]]; then
    cat <<'HELP'
lmon 2.0 — минимальный монитор сети и нагрузки.

Заголовок закреплён вверху окна, ниже идут строки значений.
При изменении размера терминала число строк подстраивается,
а заголовок остаётся на месте.

Колонки:
  N  — пинг до внешнего хоста (LMON_TARGET, по умолчанию 8.8.8.8), мс.
       Показывает качество канала до интернета.
  G  — пинг до шлюза (LMON_GW, по умолчанию 192.168.0.1), мс.
       Если N и G прыгают вместе — проблема локальная (Wi-Fi/роутер).
       Если только N — внешний маршрут или провайдер.
  L  — load average, целая часть. Загрузка системы.
  M  — занятая память, % (знак % указан в заголовке).
  T  — температура CPU, °C.

Цвета: зелёный — норма, жёлтый — внимание, красный — плохо.
Пороги:
  N: <=50 мс — зелёный, <=150 — жёлтый, >150 — красный
  G: <=10 мс — зелёный, <=50 — жёлтый, >50 — красный
  L: <=4 — зелёный, <=8 — жёлтый, >8 — красный
  M: <=60% — зелёный, <=85% — жёлтый, >85% — красный
  T: <=60°C — зелёный, <=75°C — жёлтый, >75°C — красный

Использование:
  lmon             — бесконечно, примерно раз в секунду
  lmon 20          — 20 строк и выход
  lmon help        — эта справка

Настройки (переменные окружения):
  LMON_TARGET      внешний хост (по умолчанию 8.8.8.8)
  LMON_GW          шлюз (по умолчанию 192.168.0.1)
  LMON_INTERVAL    пауза между строками, сек (по умолчанию 1)

Примеры:
  LMON_TARGET=1.1.1.1 lmon
  LMON_GW=192.168.0.1 LMON_INTERVAL=2 lmon 10

Первая строка после запуска может быть завышена: Wi-Fi-радио
просыпается. Дальше значения стабилизируются.

Температура (колонка T) требует установленной утилиты smctemp
(https://github.com/holtmann/osx/tree/master/smctemp); без неё — 0.
HELP
    return 0
  fi

  local -i iters=0 it=0
  [[ "$1" == <-> ]] && iters=$1
  local RESET=$'\e[0m' GRN=$'\e[1;32m' YEL=$'\e[1;33m' RED=$'\e[1;31m' DIM=$'\e[2m'
  local target=${LMON_TARGET:-8.8.8.8}
  local gw=${LMON_GW:-192.168.0.1}
  local p1 p2 la mem temp
  local -i rows=0 r

  # на выходе (в т.ч. по Ctrl+C) вернуть нормальный режим терминала
  trap 'printf "\e[r\e[?25h"; trap - INT' INT

  # два пакета: первый будит Wi-Fi, второй — честная задержка
  _p() {
    local v
    v=$(ping -c 2 -i 0.1 -W 1000 "$1" 2>/dev/null | awk -F'time=' '/time=/{v=$2} END{printf "%d", v}')
    print -r -- "${v:-999}"
  }

  _cell() { # значение, зелёный порог, жёлтый порог, ширина
    local -i v=$1 g=$2 y=$3 w=$4
    local s
    s=$(printf "%${w}s" "$v")
    if (( v<=g )); then print -n "$GRN$s$RESET"
    elif (( v<=y )); then print -n "$YEL$s$RESET"
    else print -n "$RED$s$RESET"; fi
  }

  _lcell() { # load average — только целая часть
    local v
    local s i
    i=${v%%.*}
    [[ -z "$i" ]] && i=0
    s=$(printf "%3s" "$i")
    if (( i<=4 )); then print -n "$GRN$s$RESET"
    elif (( i<=8 )); then print -n "$YEL$s$RESET"
    else print -n "$RED$s$RESET"; fi
  }

  _tcell() { # температура — значение со знаком градуса
    local -i v=$1 g=$2 y=$3
    local s
    s="$(printf "%3s" "$v")°"
    if (( v<=g )); then print -n "$GRN$s$RESET"
    elif (( v<=y )); then print -n "$YEL$s$RESET"
    else print -n "$RED$s$RESET"; fi
  }

  _setup() {
    r=${LINES:-24}
    (( r < 4 )) && r=4
    printf '\e[2J\e[H'
    printf '%s%3s %3s %3s %3s %4s%s\n' "$DIM" N G L 'M%' 'T°' "$RESET"
    printf '%s%s%s\n' "$DIM" '--- --- --- --- ----' "$RESET"
    printf '\e[3;%dr' "$r"
    printf '\e[%d;1H' "$r"
    rows=$r
  }

  printf '\e[?25l'   # спрятать курсор — не мелькает
  _setup

  while true; do
    p1=$(_p "$target"); p2=$(_p "$gw")
    la=$(uptime | awk -F'load averages?: ' '{print $2}' | awk '{print $1}')
    [[ -z "$la" ]] && la=0
    mem=$(vm_stat | awk '/free/{f=$3}/active/{a=$3}/inactive/{i=$3}/speculative/{s=$3}/wired/{w=$4}/compressed/{c=$3}END{gsub(/\./,"",f);gsub(/\./,"",a);gsub(/\./,"",i);gsub(/\./,"",s);gsub(/\./,"",w);gsub(/\./,"",c);t=f+a+i+s+w+c;print t?int((a+w+c)/t*100):0}')
    temp=$(smctemp -c 2>/dev/null | sed -nE 's/([0-9]+).*/\1/p'); temp=${temp:-0}

    r=${LINES:-24}
    (( r < 4 )) && r=4
    (( r != rows )) && _setup

    printf '\r\n'   # прокрутка только области под заголовком
    _cell $p1 50 150 3
    printf ' '
    _cell $p2 10 50 3
    printf ' '
    _lcell $la
    printf ' '
    _cell $mem 60 85 3
    printf ' '
    _tcell $temp 60 75

    (( iters>0 )) && (( ++it >= iters )) && break
    sleep ${LMON_INTERVAL:-1}
  done

  trap - INT
  printf '\e[r\e[?25h\n'
}

lmon "$@"
