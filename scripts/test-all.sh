#!/bin/sh
# Локальная проверка lmon вместо CI (пока GitHub Actions недоступен).
# Матрица:
#   macOS  — gofmt, go vet, go test, go test -race (нативно)
#   Linux  — go vet + go test в контейнере golang:1.22-alpine (docker/colima)
#   arm64/amd64 darwin/linux + windows — кросс-сборка (CGO_ENABLED=0)
set -eu
cd "$(dirname "$0")/.."

fail=0
step() { printf '\n=== %s\n' "$1"; }

step "gofmt (macOS)"
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "НЕОТФОРМАТИРОВАНО: $unformatted"; fail=1
else echo ok; fi

step "go vet + test + race (macOS, нативно)"
go vet ./... || fail=1
go test ./... || fail=1
go test -race ./... || fail=1

step "go vet + test (Linux, контейнер golang:1.22-alpine через colima)"
if docker info >/dev/null 2>&1; then
  docker run --rm -v "$PWD":/src -w /src golang:1.22-alpine sh -c \
    'go vet ./... && go test ./...' || fail=1
  # amd64 не гоняем под qemu: эмуляция нестабильна с Go-ассемблером,
  # а компиляцию под эту архитектуру проверяет кросс-сборка ниже
else
  echo "НЕТ DOCKER: запусти colima start"; fail=1
fi

step "сайт docs/: синтаксис JS и целостность HTML"
if command -v node >/dev/null 2>&1; then
  for f in docs/index.html docs/en/index.html; do
    node -e "
      const fs = require('fs');
      const html = fs.readFileSync(process.argv[1], 'utf8');
      const scripts = [...html.matchAll(/<script>([\\s\\S]*?)<\\/script>/g)];
      if (scripts.length === 0) { console.error('нет script-блоков: ' + process.argv[1]); process.exit(1); }
      for (const m of scripts) { try { new Function(m[1]); } catch (e) { console.error(process.argv[1] + ': ' + e.message); process.exit(1); } }
    " "$f" || fail=1
    grep -q 'id="screen"' "$f" && grep -q 'function tick' "$f" || { echo "НЕТ ДЕМКИ: $f"; fail=1; }
    echo "ok: $f"
  done
else
  echo "нет node — проверка сайта пропущена"
fi

step "кросс-сборка (статика, все платформы)"
for target in \
  darwin/arm64 darwin/amd64 \
  linux/arm64 linux/amd64 \
  windows/arm64 windows/amd64
do
  GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 \
    go build -o /dev/null . || { echo "СБОЙ: $target"; fail=1; }
  echo "ok: $target"
done

if [ "$fail" -ne 0 ]; then
  echo '\n=== ЕСТЬ ОШИБКИ'; exit 1
fi
echo '\n=== ВСЁ ЗЕЛЁНОЕ'
