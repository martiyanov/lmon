#!/bin/sh
# Обновляет формулу lmon в репозитории homebrew-tap по вышедшему релизу.
# Запуск после публикации тега и завершения Release-воркфлоу:
#   scripts/update-tap.sh v1.0.3 [путь-к-клону-homebrew-tap]
set -eu

tag="${1:?использование: update-tap.sh <тег, напр. v1.0.3>}"
tap_dir="${2:-$HOME/Проекты/homebrew-tap}"
ver="${tag#v}"
base="https://github.com/martiyanov/lmon/releases/download/$tag"

sums=$(curl -fsSL "$base/checksums.txt")
sha() { printf '%s\n' "$sums" | awk -v f="$1" '$2==f{print $1}'; }

cd "$tap_dir"
git pull -q

cat > Formula/lmon.rb <<FORMULA
class Lmon < Formula
  desc "Minimal terminal monitor: ping, load average, memory, CPU temperature"
  homepage "https://github.com/martiyanov/lmon"
  version "$ver"
  license "MIT"

  on_macos do
    if Hardware::CPU.intel?
      url "$base/lmon_darwin_amd64.tar.gz"
      sha256 "$(sha lmon_darwin_amd64.tar.gz)"
    end
    if Hardware::CPU.arm?
      url "$base/lmon_darwin_arm64.tar.gz"
      sha256 "$(sha lmon_darwin_arm64.tar.gz)"
    end
  end

  on_linux do
    if Hardware::CPU.intel?
      url "$base/lmon_linux_amd64.tar.gz"
      sha256 "$(sha lmon_linux_amd64.tar.gz)"
    end
    if Hardware::CPU.arm?
      url "$base/lmon_linux_arm64.tar.gz"
      sha256 "$(sha lmon_linux_arm64.tar.gz)"
    end
  end

  def install
    bin.install "lmon"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/lmon --version")
  end
end
FORMULA

git add Formula/lmon.rb
git commit -q -m "lmon $ver formula"
git push -q
echo "Формула lmon $ver обновлена в $tap_dir"
