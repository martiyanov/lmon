#!/bin/sh
# Локальный релиз lmon без GitHub Actions:
#   1) полная локальная матрица (scripts/test-all.sh — macOS + Linux в Colima);
#   2) сборка в чистом клоне на теге (goreleaser), чтобы не зависеть от
#      грязного рабочего дерева и очереди Actions;
#   3) публикация релиза и ассетов через gh;
#   4) обновление Homebrew-тапа.
#
# Использование:
#   scripts/release-local.sh v1.2.3 [--notes-file FILE] [--skip-ci]
#                                  [--no-tap] [--update] [--dry-run]
#
# Требуется: git, gh (авторизован, scope repo), goreleaser, docker/colima
# (только для CI; с --skip-ci не нужен). Тег должен быть создан и запушен
# заранее — скрипт его не двигает.
set -eu
cd "$(dirname "$0")/.."

usage() {
	cat <<EOF
usage: $0 <tag> [options]

  <tag>            существующий запушенный тег, например v1.2.3
  --notes-file F   текст релиза из файла (иначе — аннотация тега)
  --skip-ci        не гонять scripts/test-all.sh (быстрый повторный релиз)
  --no-tap         не обновлять Homebrew-тап
  --update         релиз уже есть: перезалить ассеты (--clobber)
  --dry-run        только проверки, без сборки и публикации
EOF
}

tag=""
notes=""
skip_ci=0
no_tap=0
update=0
dry=0
while [ $# -gt 0 ]; do
	case "$1" in
	--notes-file)
		[ $# -ge 2 ] || { echo "--notes-file требует путь" >&2; exit 2; }
		notes=$2
		shift 2
		;;
	--skip-ci) skip_ci=1; shift ;;
	--no-tap) no_tap=1; shift ;;
	--update) update=1; shift ;;
	--dry-run) dry=1; shift ;;
	-h | --help) usage; exit 0 ;;
	-*) echo "неизвестный флаг: $1" >&2; usage; exit 2 ;;
	*)
		if [ -n "$tag" ]; then
			echo "лишний аргумент: $1" >&2
			exit 2
		fi
		tag=$1
		shift
		;;
	esac
done

[ -n "$tag" ] || { usage; exit 2; }
case "$tag" in
v*) ;;
*) echo "тег должен начинаться с v: $tag" >&2; exit 2 ;;
esac

# --- проверки окружения -------------------------------------------------
git rev-parse --verify "$tag^{commit}" >/dev/null 2>&1 || {
	echo "нет локального тега $tag" >&2
	exit 1
}
git ls-remote --tags origin "refs/tags/$tag" 2>/dev/null | grep -q . || {
	echo "тег $tag не запушен: git push origin $tag" >&2
	exit 1
}
command -v gh >/dev/null 2>&1 || { echo "нет gh (GitHub CLI)" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "сначала: gh auth login" >&2; exit 1; }
command -v goreleaser >/dev/null 2>&1 || {
	echo "нет goreleaser: brew install goreleaser" >&2
	exit 1
}
if [ -n "$notes" ] && [ ! -f "$notes" ]; then
	echo "нет файла заметок: $notes" >&2
	exit 1
fi

title=$(git tag -l --format='%(contents:subject)' "$tag" | head -1)
[ -n "$title" ] || title=$tag

if [ "$skip_ci" -eq 0 ]; then
	if ! docker info >/dev/null 2>&1; then
		echo "CI требует Docker/Colima: colima start (или --skip-ci)" >&2
		exit 1
	fi
fi

if [ "$dry" -eq 1 ]; then
	echo "dry-run: tag=$tag title=$title notes=${notes:-<tag annotation>}"
	echo "dry-run: CI=$([ "$skip_ci" -eq 1 ] && echo skip || echo scripts/test-all.sh)"
	echo "dry-run: build=goreleaser в чистом клоне; publish=gh release $([ "$update" -eq 1 ] && echo update || echo create) $tag"
	exit 0
fi

# --- CI ------------------------------------------------------------------
if [ "$skip_ci" -eq 0 ]; then
	echo "== CI: scripts/test-all.sh (macOS + Linux в Colima)"
	./scripts/test-all.sh
fi

# --- сборка в чистом клоне на теге --------------------------------------
tmp=$(mktemp -d "${TMPDIR:-/tmp}/lmon-release.XXXXXX")
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT INT TERM

echo "== clone $tag"
git clone --quiet --depth 1 --branch "$tag" \
	git@github.com:martiyanov/lmon.git "$tmp/repo"

echo "== goreleaser"
(cd "$tmp/repo" && goreleaser release --clean --skip=publish)
dist="$tmp/repo/dist"

assets=$(find "$dist" -maxdepth 1 \( \
	-name 'lmon_*.tar.gz' -o -name 'lmon_*.zip' \
	-o -name 'lmon_*.deb' -o -name 'lmon_*.rpm' \) -print | sort)
[ -n "$assets" ] || { echo "в dist нет ассетов" >&2; exit 1; }
[ -f "$dist/checksums.txt" ] || { echo "нет checksums.txt" >&2; exit 1; }

# word splitting нужен: gh ждёт список файлов
# shellcheck disable=SC2086
if gh release view "$tag" >/dev/null 2>&1; then
	if [ "$update" -ne 1 ]; then
		echo "релиз $tag уже существует; для перезалива ассетов: --update" >&2
		exit 1
	fi
	echo "== gh release upload --clobber"
	gh release upload "$tag" $assets "$dist/checksums.txt" --clobber
	if [ -n "$notes" ]; then
		gh release edit "$tag" --notes-file "$notes"
	fi
else
	echo "== gh release create"
	if [ -n "$notes" ]; then
		gh release create "$tag" $assets "$dist/checksums.txt" \
			--title "$title" --notes-file "$notes"
	else
		gh release create "$tag" $assets "$dist/checksums.txt" \
			--title "$title" --notes-from-tag
	fi
fi

echo "== release $tag"
gh release view "$tag" --json url,assets --jq '"\(.url) — \(.assets|length) ассетов"'

if [ "$no_tap" -eq 0 ]; then
	echo "== Homebrew tap"
	./scripts/update-tap.sh "$tag"
fi

echo "== готово: $tag"
